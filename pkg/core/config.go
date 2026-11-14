package core

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Deployment modes stored on a target. Junction is a recorded fallback, not
// a configured mode.
const (
	ModeSymlink  = "symlink"
	ModeCopy     = "copy"
	ModeJunction = "junction"
)

// Skill file formats.
const (
	FormatSkill      = "skill"
	FormatGeminiTOML = "gemini-toml"
)

// TargetsSchemaVersion is the version written into new targets.json files.
const TargetsSchemaVersion = 1

// Target is one tool Skills Manager can deploy into.
type Target struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Format  string `json:"format"`
	Builtin bool   `json:"builtin,omitempty"`
	Docs    string `json:"docs,omitempty"`
}

type targetsDocument struct {
	Version int      `json:"version"`
	Checked string   `json:"checked,omitempty"`
	Targets []Target `json:"targets"`
}

//go:embed targets/catalog.json
var defaultCatalog []byte

var targetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// DefaultTargets returns a copy of the versioned built-in catalog.
func DefaultTargets() []Target {
	doc, err := parseCatalog(defaultCatalog)
	if err != nil {
		return nil
	}
	out := make([]Target, len(doc.Targets))
	copy(out, doc.Targets)
	return out
}

// CatalogChecked is the date the built-in paths were compared to upstream docs.
func CatalogChecked() string {
	doc, err := parseCatalog(defaultCatalog)
	if err != nil {
		return ""
	}
	return doc.Checked
}

func parseCatalog(data []byte) (targetsDocument, error) {
	var doc targetsDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return targetsDocument{}, err
	}
	return doc, nil
}

// LoadTargets reads targets.json. A missing file is created from the catalog.
// A legacy JSON array is accepted. Paths that do not expand are kept and
// reported as warnings so one bad target does not block the others.
func LoadTargets(filePath string) ([]Target, []string, error) {
	clean, err := ExpandPath(filePath)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		if os.IsNotExist(err) {
			defaults := DefaultTargets()
			if werr := SaveTargets(clean, defaults); werr != nil {
				return defaults, nil, fmt.Errorf("write default targets: %w", werr)
			}
			return defaults, nil, nil
		}
		return nil, nil, fmt.Errorf("read targets %s: %w", clean, err)
	}
	targets, err := decodeTargets(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse targets %s: %w", clean, err)
	}
	var warnings []string
	catalog := map[string]Target{}
	for _, t := range DefaultTargets() {
		catalog[t.ID] = t
	}
	for i := range targets {
		if err := normalizeTarget(&targets[i]); err != nil {
			return nil, nil, err
		}
		if cat, ok := catalog[targets[i].ID]; ok {
			targets[i].Builtin = true
			if targets[i].Docs == "" {
				targets[i].Docs = cat.Docs
			}
			if targets[i].Path != cat.Path {
				warnings = append(warnings, fmt.Sprintf("built-in target %q path %q differs from catalog %q (checked %s)", targets[i].ID, targets[i].Path, cat.Path, CatalogChecked()))
			}
		}
		if _, err := ExpandPath(targets[i].Path); err != nil {
			warnings = append(warnings, fmt.Sprintf("target %q path: %v", targets[i].ID, err))
		}
	}
	if err := inspectTargets(targets); err != nil {
		warnings = append(warnings, err.Error())
	}
	return targets, warnings, nil
}

func decodeTargets(data []byte) ([]Target, error) {
	trim := strings.TrimSpace(string(data))
	if trim == "" {
		return nil, fmt.Errorf("%w: targets file is empty", ErrCorrupt)
	}
	if trim[0] == '[' {
		var targets []Target
		if err := json.Unmarshal(data, &targets); err != nil {
			return nil, err
		}
		return targets, nil
	}
	var doc targetsDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Version > TargetsSchemaVersion {
		return nil, fmt.Errorf("%w: targets version %d is newer than supported %d", ErrCorrupt, doc.Version, TargetsSchemaVersion)
	}
	return doc.Targets, nil
}

func normalizeTarget(t *Target) error {
	t.ID = strings.TrimSpace(t.ID)
	t.Path = strings.TrimSpace(t.Path)
	t.Label = strings.TrimSpace(t.Label)
	if !targetIDPattern.MatchString(t.ID) {
		return fmt.Errorf("%w: target id %q", ErrInvalidName, t.ID)
	}
	if t.Path == "" {
		return fmt.Errorf("%w: target %s has an empty path", ErrInvalidPath, t.ID)
	}
	if t.Mode == "" {
		t.Mode = ModeSymlink
	}
	if t.Format == "" {
		t.Format = FormatSkill
	}
	if t.Mode != ModeSymlink && t.Mode != ModeCopy {
		return fmt.Errorf("%w: target %s mode %q", ErrInvalidPath, t.ID, t.Mode)
	}
	if t.Format != FormatSkill && t.Format != FormatGeminiTOML {
		return fmt.Errorf("%w: target %s format %q", ErrInvalidPath, t.ID, t.Format)
	}
	if t.Label == "" {
		t.Label = t.ID
	}
	return nil
}

// ValidateTarget checks id, mode, format, and that path expands to an absolute path.
func ValidateTarget(t Target) error {
	cp := t
	if err := normalizeTarget(&cp); err != nil {
		return err
	}
	expanded, err := ExpandPath(cp.Path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(expanded) {
		return fmt.Errorf("%w: target %s path %q is not absolute", ErrInvalidPath, cp.ID, expanded)
	}
	return nil
}

// inspectTargets checks each target and rejects duplicate ids or overlapping paths.
// It does not know the canonical store; validateTargetSet adds that.
func inspectTargets(targets []Target) error {
	seen := map[string]struct{}{}
	type expanded struct {
		id   string
		path string
	}
	var paths []expanded
	for _, t := range targets {
		if err := ValidateTarget(t); err != nil {
			return err
		}
		cp := t
		if err := normalizeTarget(&cp); err != nil {
			return err
		}
		if _, ok := seen[cp.ID]; ok {
			return fmt.Errorf("%w: duplicate target id %q", ErrInvalidName, cp.ID)
		}
		seen[cp.ID] = struct{}{}
		path, err := ExpandPath(cp.Path)
		if err != nil {
			return err
		}
		for _, prev := range paths {
			if pathsOverlap(path, prev.path) {
				return fmt.Errorf("%w: target %s path overlaps target %s", ErrInvalidPath, cp.ID, prev.id)
			}
		}
		paths = append(paths, expanded{id: cp.ID, path: path})
	}
	return nil
}

// validateTargetSet is inspectTargets plus a refusal to deploy into the
// canonical store or the config directory.
func validateTargetSet(canonical, config string, targets []Target) error {
	if err := inspectTargets(targets); err != nil {
		return err
	}
	for _, t := range targets {
		cp := t
		if err := normalizeTarget(&cp); err != nil {
			return err
		}
		path, err := ExpandPath(cp.Path)
		if err != nil {
			return err
		}
		if pathsOverlap(path, canonical) {
			return fmt.Errorf("%w: target %s path overlaps the canonical store", ErrInvalidPath, cp.ID)
		}
		if config != "" && pathsOverlap(path, config) {
			return fmt.Errorf("%w: target %s path overlaps the config directory", ErrInvalidPath, cp.ID)
		}
	}
	return nil
}

// rejectUnsafeTarget refuses to deploy through one target that is relative,
// overlaps another configured target, or overlaps the store or config directory.
func (m *Manager) rejectUnsafeTarget(target Target) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}
	cp := target
	if err := normalizeTarget(&cp); err != nil {
		return err
	}
	path, err := ExpandPath(cp.Path)
	if err != nil {
		return err
	}
	if pathsOverlap(path, m.CanonicalDir) {
		return fmt.Errorf("%w: target %s path overlaps the canonical store", ErrInvalidPath, cp.ID)
	}
	if pathsOverlap(path, m.ConfigDir) {
		return fmt.Errorf("%w: target %s path overlaps the config directory", ErrInvalidPath, cp.ID)
	}
	for _, other := range m.targets {
		if other.ID == cp.ID {
			continue
		}
		otherPath, err := ExpandPath(other.Path)
		if err != nil {
			continue
		}
		if pathsOverlap(path, otherPath) {
			return fmt.Errorf("%w: target %s path overlaps target %s", ErrInvalidPath, cp.ID, other.ID)
		}
	}
	return nil
}

// SaveTargets writes targets as a versioned document, atomically.
func SaveTargets(filePath string, targets []Target) error {
	clean, err := ExpandPath(filePath)
	if err != nil {
		return err
	}
	for i := range targets {
		if err := normalizeTarget(&targets[i]); err != nil {
			return err
		}
	}
	doc := targetsDocument{Version: TargetsSchemaVersion, Checked: CatalogChecked(), Targets: targets}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeAtomic(clean, data, 0o644); err != nil {
		return fmt.Errorf("write targets %s: %w", clean, err)
	}
	return nil
}

// GetTargetDestinationPath returns the file or directory a skill occupies
// inside a target. The skill name must be valid.
func GetTargetDestinationPath(target Target, skillName string) (string, error) {
	if err := ValidateSkillName(skillName); err != nil {
		return "", err
	}
	dir, err := ExpandPath(target.Path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%w: target %s path %q is not absolute", ErrInvalidPath, target.ID, dir)
	}
	name := skillName
	if target.Format == FormatGeminiTOML {
		name += ".toml"
	}
	dest := joinUnder(dir, name)
	if err := ensureWithin(dir, dest); err != nil {
		return "", err
	}
	if err := hermeticGuard(dest); err != nil {
		return "", err
	}
	return dest, nil
}

func joinUnder(dir, name string) string {
	return filepath.Join(dir, name)
}
