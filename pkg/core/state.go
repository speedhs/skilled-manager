package core

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// StateSchemaVersion is the on-disk version written by this build.
// Version 0 is the original file that had no version field.
const StateSchemaVersion = 1

// TargetRecord is the proof that this tool created a deployment.
// Device and Inode identify the directory entry. Hash is the deployed
// content hash. CanonicalHash is the canonical skill hash at that time.
type TargetRecord struct {
	Mode          string `json:"mode"`
	Format        string `json:"format"`
	Hash          string `json:"hash"`
	CanonicalHash string `json:"canonical_hash,omitempty"`
	Path          string `json:"path"`
	UpdatedAt     string `json:"updated_at"`
	Fallback      bool   `json:"fallback,omitempty"`
	Device        uint64 `json:"device,omitempty"`
	Inode         uint64 `json:"inode,omitempty"`
	IdentityValid bool   `json:"identity_valid,omitempty"`
}

// State is the set of deployments this tool created.
type State struct {
	mu      sync.RWMutex
	Version int                                `json:"version"`
	Targets map[string]map[string]TargetRecord `json:"targets"`
}

// NewState returns an empty current-version state.
func NewState() *State {
	return &State{Version: StateSchemaVersion, Targets: map[string]map[string]TargetRecord{}}
}

// LoadState reads state.json.
// A missing file yields an empty state. A file this build cannot parse is
// renamed aside and an empty state is returned with a warning. A newer
// schema version is an error so it is not overwritten.
func LoadState(filePath string) (*State, string, error) {
	clean, err := ExpandPath(filePath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), "", nil
		}
		return nil, "", fmt.Errorf("read state %s: %w", clean, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		backup := fmt.Sprintf("%s.corrupt-%s", clean, time.Now().UTC().Format("20060102T150405.000000000Z"))
		if renErr := os.Rename(clean, backup); renErr != nil {
			return nil, "", fmt.Errorf("%w: %s (%v); also failed to quarantine: %v", ErrCorrupt, clean, err, renErr)
		}
		return NewState(), fmt.Sprintf("state file was corrupt and moved to %s", backup), nil
	}
	if s.Version > StateSchemaVersion {
		return nil, "", fmt.Errorf("%w: state version %d is newer than supported %d", ErrCorrupt, s.Version, StateSchemaVersion)
	}
	if s.Targets == nil {
		s.Targets = map[string]map[string]TargetRecord{}
	}
	// Version 0 files gain a version on the next save. Nothing else moves.
	if s.Version < StateSchemaVersion {
		s.Version = StateSchemaVersion
	}
	return &s, "", nil
}

// Save writes state atomically (temp file, fsync, rename).
func (s *State) Save(filePath string) error {
	return s.SaveTo(osFS{}, filePath)
}

// SaveTo writes state through fsys so tests can fail the write.
func (s *State) SaveTo(fsys fileSystem, filePath string) error {
	clean, data, err := s.snapshot(filePath)
	if err != nil {
		return err
	}
	if err := fsys.WriteFileAtomic(clean, data, 0o644); err != nil {
		return fmt.Errorf("write state %s: %w", clean, err)
	}
	return nil
}

func (s *State) snapshot(filePath string) (string, []byte, error) {
	s.mu.RLock()
	version := s.Version
	if version == 0 {
		version = StateSchemaVersion
	}
	targets := make(map[string]map[string]TargetRecord, len(s.Targets))
	for id, skills := range s.Targets {
		inner := make(map[string]TargetRecord, len(skills))
		for name, rec := range skills {
			inner[name] = rec
		}
		targets[id] = inner
	}
	s.mu.RUnlock()
	clean, err := ExpandPath(filePath)
	if err != nil {
		return "", nil, err
	}
	payload := struct {
		Version int                                `json:"version"`
		Targets map[string]map[string]TargetRecord `json:"targets"`
	}{Version: version, Targets: targets}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", nil, err
	}
	return clean, append(data, '\n'), nil
}

// GetRecord returns the record for a target and skill.
func (s *State) GetRecord(targetID, skillName string) (TargetRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.Targets[targetID][skillName]
	return rec, ok
}

// SetRecord inserts or replaces a record and stamps UpdatedAt when empty.
func (s *State) SetRecord(targetID, skillName string, record TargetRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Targets == nil {
		s.Targets = map[string]map[string]TargetRecord{}
	}
	if _, ok := s.Targets[targetID]; !ok {
		s.Targets[targetID] = map[string]TargetRecord{}
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	s.Targets[targetID][skillName] = record
}

// DeleteRecord removes one record.
func (s *State) DeleteRecord(targetID, skillName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.Targets[targetID]; ok {
		delete(m, skillName)
		if len(m) == 0 {
			delete(s.Targets, targetID)
		}
	}
}

// IsTracked reports whether a record exists.
func (s *State) IsTracked(targetID, skillName string) bool {
	_, ok := s.GetRecord(targetID, skillName)
	return ok
}

// identity of a record, when one was stored.
func recordID(rec TargetRecord) FileID {
	if !rec.IdentityValid {
		return FileID{}
	}
	return FileID{Dev: rec.Device, Ino: rec.Inode, Valid: true}
}
