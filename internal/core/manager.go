package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultCanonicalDir = "~/.agent-skills"
	DefaultConfigDir    = "~/.agent-skills-manager"
	TargetsFilename     = "targets.json"
	StateFilename       = "state.json"
)

// Manager coordinates skills, targets, state, and synchronization.
type Manager struct {
	mu           sync.RWMutex
	CanonicalDir string
	ConfigDir    string
	TargetsPath  string
	StatePath    string
	targets      []Target
	state        *State
}

// Option configures a Manager instance.
type Option func(*Manager)

// WithCanonicalDir overrides the canonical skills directory.
func WithCanonicalDir(dir string) Option {
	return func(m *Manager) {
		m.CanonicalDir = ExpandPath(dir)
	}
}

// WithConfigDir overrides the configuration directory.
func WithConfigDir(dir string) Option {
	return func(m *Manager) {
		m.ConfigDir = ExpandPath(dir)
		m.TargetsPath = filepath.Join(m.ConfigDir, TargetsFilename)
		m.StatePath = filepath.Join(m.ConfigDir, StateFilename)
	}
}

// NewManager creates an initialized Manager with options applied.
func NewManager(opts ...Option) (*Manager, error) {
	m := &Manager{
		CanonicalDir: ExpandPath(DefaultCanonicalDir),
		ConfigDir:    ExpandPath(DefaultConfigDir),
	}
	m.TargetsPath = filepath.Join(m.ConfigDir, TargetsFilename)
	m.StatePath = filepath.Join(m.ConfigDir, StateFilename)

	for _, opt := range opts {
		opt(m)
	}

	// Ensure directories exist
	if err := os.MkdirAll(m.CanonicalDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create canonical dir %s: %w", m.CanonicalDir, err)
	}
	if err := os.MkdirAll(m.ConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config dir %s: %w", m.ConfigDir, err)
	}

	// Load targets and state
	if err := m.Reload(); err != nil {
		return nil, err
	}

	return m, nil
}

// Reload reloads targets and state from disk.
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	targets, err := LoadTargets(m.TargetsPath)
	if err != nil {
		return fmt.Errorf("failed to load targets: %w", err)
	}
	m.targets = targets

	state, err := LoadState(m.StatePath)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	m.state = state

	return nil
}

// GetTargets returns a copy of the current target configurations.
func (m *Manager) GetTargets() []Target {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Target, len(m.targets))
	copy(result, m.targets)
	return result
}

// GetTargetByID finds a target by its unique ID.
func (m *Manager) GetTargetByID(id string) (Target, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, t := range m.targets {
		if t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}

// UpdateTargets updates the target configurations and saves to disk.
func (m *Manager) UpdateTargets(targets []Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := SaveTargets(m.TargetsPath, targets); err != nil {
		return err
	}
	m.targets = targets
	return nil
}

// GetState returns the current state instance.
func (m *Manager) GetState() *State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// SaveState persists the state to disk.
func (m *Manager) SaveState() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Save(m.StatePath)
}
