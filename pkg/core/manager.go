package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	// DefaultCanonicalDir is the skill library used when no option overrides it.
	DefaultCanonicalDir = "~/.agent-skills"
	// DefaultConfigDir holds targets.json, state.json, the lock, and backups.
	DefaultConfigDir = "~/.agent-skills-manager"
	// TargetsFilename is the config file name inside the config directory.
	TargetsFilename = "targets.json"
	// StateFilename is the deployment ledger inside the config directory.
	StateFilename = "state.json"
)

// Manager coordinates skills, targets, and the deployment ledger.
//
// Mutating methods take the process lock and an exclusive file lock.
// Status does not take the file lock. Do not call a mutating method from
// a hook while a mutation is already running on the same Manager.
type Manager struct {
	mu           sync.RWMutex
	CanonicalDir string
	ConfigDir    string
	TargetsPath  string
	StatePath    string
	targets      []Target
	state        *State
	fs           fileSystem
	logger       *slog.Logger
	dryRun       bool
	warnings     []string
}

// Option configures NewManager.
type Option func(*Manager)

// WithCanonicalDir sets the canonical skill directory. The path is expanded
// when the manager is created.
func WithCanonicalDir(dir string) Option {
	return func(m *Manager) {
		m.CanonicalDir = dir
	}
}

// WithConfigDir sets the directory that holds targets, state, and backups.
func WithConfigDir(dir string) Option {
	return func(m *Manager) {
		m.ConfigDir = dir
	}
}

// WithLogger sets the logger used for operational messages.
// A nil logger is ignored. The default logger discards everything.
func WithLogger(logger *slog.Logger) Option {
	return func(m *Manager) {
		if logger != nil {
			m.logger = logger
		}
	}
}

// WithDryRun makes mutating methods report what they would do and write nothing.
func WithDryRun(dry bool) Option {
	return func(m *Manager) {
		m.dryRun = dry
	}
}

// NewManager opens or creates the canonical and config directories and loads
// targets and state.
func NewManager(opts ...Option) (*Manager, error) {
	m := &Manager{
		CanonicalDir: DefaultCanonicalDir,
		ConfigDir:    DefaultConfigDir,
		fs:           osFS{},
		logger:       discardLogger(),
		state:        NewState(),
	}
	for _, opt := range opts {
		opt(m)
	}
	var err error
	m.CanonicalDir, err = ExpandPath(m.CanonicalDir)
	if err != nil {
		return nil, err
	}
	m.ConfigDir, err = ExpandPath(m.ConfigDir)
	if err != nil {
		return nil, err
	}
	if err := hermeticGuard(m.CanonicalDir); err != nil {
		return nil, err
	}
	if err := hermeticGuard(m.ConfigDir); err != nil {
		return nil, err
	}
	m.TargetsPath = filepath.Join(m.ConfigDir, TargetsFilename)
	m.StatePath = filepath.Join(m.ConfigDir, StateFilename)
	if err := os.MkdirAll(m.CanonicalDir, 0o755); err != nil {
		return nil, fmt.Errorf("create canonical dir: %w", err)
	}
	if err := os.MkdirAll(m.ConfigDir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	if err := m.reloadLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) files() fileSystem {
	if m.fs == nil {
		return osFS{}
	}
	return m.fs
}

func (m *Manager) log() *slog.Logger {
	if m.logger == nil {
		return discardLogger()
	}
	return m.logger
}

// Warnings returns notes from the last load, such as a quarantined state file
// or a built-in target whose path differs from the catalog.
func (m *Manager) Warnings() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.warnings))
	copy(out, m.warnings)
	return out
}

// DryRun reports whether mutating methods will avoid writing.
func (m *Manager) DryRun() bool {
	return m.dryRun
}

// Reload rereads targets and state from disk.
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reloadLocked()
}

func (m *Manager) reloadLocked() error {
	targets, warnings, err := LoadTargets(m.TargetsPath)
	if err != nil {
		return err
	}
	state, warning, err := LoadState(m.StatePath)
	if err != nil {
		return err
	}
	m.targets = targets
	m.state = state
	m.warnings = append([]string{}, warnings...)
	if warning != "" {
		m.warnings = append(m.warnings, warning)
		m.log().Warn(warning)
	}
	return nil
}

// GetTargets returns a copy of the configured targets.
func (m *Manager) GetTargets() []Target {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Target, len(m.targets))
	copy(out, m.targets)
	return out
}

// GetTargetByID returns the target with id.
func (m *Manager) GetTargetByID(id string) (Target, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.targetByID(id)
}

func (m *Manager) targetByID(id string) (Target, bool) {
	for _, t := range m.targets {
		if t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}

// UpdateTargets validates, saves, and stores targets.
func (m *Manager) UpdateTargets(targets []Target) error {
	return m.mutate(context.Background(), func() error {
		for i := range targets {
			if err := ValidateTarget(targets[i]); err != nil {
				return err
			}
		}
		if m.dryRun {
			return nil
		}
		if err := SaveTargets(m.TargetsPath, targets); err != nil {
			return err
		}
		m.targets = append([]Target(nil), targets...)
		return nil
	})
}

// GetState returns the in-memory ledger. Callers must treat it as read-only
// while any mutation may be running.
func (m *Manager) GetState() *State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// SaveState writes the current ledger.
func (m *Manager) SaveState() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Save(m.StatePath)
}

func (m *Manager) mutate(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	release, err := m.acquireLock()
	if err != nil {
		return err
	}
	defer release()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

func (m *Manager) acquireLock() (func(), error) {
	if err := os.MkdirAll(m.ConfigDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(m.ConfigDir, "manager.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, ErrLocked) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}
