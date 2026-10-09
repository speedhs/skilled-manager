package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TargetRecord tracks state for a specific skill deployed to a target.
type TargetRecord struct {
	Mode      string `json:"mode"`       // e.g. "symlink", "junction", "copy", "gemini-toml"
	Format    string `json:"format"`     // e.g. "skill", "gemini-toml"
	Hash      string `json:"hash"`       // Hash of content when synced
	Path      string `json:"path"`       // Exact target destination path
	UpdatedAt string `json:"updated_at"` // RFC3339 timestamp
	Fallback  bool   `json:"fallback,omitempty"` // true if symlink fell back to junction or copy
}

// State stores state across all targets and skills.
type State struct {
	mu      sync.RWMutex
	Targets map[string]map[string]TargetRecord `json:"targets"` // targetID -> skillName -> record
}

// NewState creates an empty State object.
func NewState() *State {
	return &State{
		Targets: make(map[string]map[string]TargetRecord),
	}
}

// LoadState loads state from state.json, or creates an empty state if not found.
func LoadState(filePath string) (*State, error) {
	cleanPath := ExpandPath(filePath)
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		return NewState(), nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file %s: %w", cleanPath, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse state JSON from %s: %w", cleanPath, err)
	}

	if s.Targets == nil {
		s.Targets = make(map[string]map[string]TargetRecord)
	}

	return &s, nil
}

// Save saves state to state.json.
func (s *State) Save(filePath string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPath := ExpandPath(filePath)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state JSON: %w", err)
	}

	if err := os.WriteFile(cleanPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", cleanPath, err)
	}

	return nil
}

// GetRecord returns the record for (targetID, skillName) if it exists.
func (s *State) GetRecord(targetID, skillName string) (TargetRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if targetMap, ok := s.Targets[targetID]; ok {
		rec, exists := targetMap[skillName]
		return rec, exists
	}
	return TargetRecord{}, false
}

// SetRecord updates or creates the record for (targetID, skillName).
func (s *State) SetRecord(targetID, skillName string, record TargetRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Targets == nil {
		s.Targets = make(map[string]map[string]TargetRecord)
	}
	if _, ok := s.Targets[targetID]; !ok {
		s.Targets[targetID] = make(map[string]TargetRecord)
	}

	if record.UpdatedAt == "" {
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	s.Targets[targetID][skillName] = record
}

// DeleteRecord removes the record for (targetID, skillName).
func (s *State) DeleteRecord(targetID, skillName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if targetMap, ok := s.Targets[targetID]; ok {
		delete(targetMap, skillName)
		if len(targetMap) == 0 {
			delete(s.Targets, targetID)
		}
	}
}

// IsTracked returns true if (targetID, skillName) has an entry in state.
func (s *State) IsTracked(targetID, skillName string) bool {
	_, exists := s.GetRecord(targetID, skillName)
	return exists
}
