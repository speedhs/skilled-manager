package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is one canonical skill folder.
type Skill struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Path        string         `json:"path"`
	Content     string         `json:"content"`
	Body        string         `json:"body"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// ListSkills returns the valid skills in canonicalDir, sorted by name.
// Folders that fail validation are skipped.
func ListSkills(canonicalDir string) ([]Skill, error) {
	clean, err := ExpandPath(canonicalDir)
	if err != nil {
		return nil, err
	}
	if err := hermeticGuard(clean); err != nil {
		return nil, err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(clean, 0o755); err != nil {
				return nil, fmt.Errorf("create canonical dir: %w", err)
			}
			return []Skill{}, nil
		}
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: canonical path is not a directory", ErrInvalidPath)
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, entry := range entries {
		if err := ValidateSkillName(entry.Name()); err != nil {
			continue
		}
		skill, err := ParseSkill(filepath.Join(clean, entry.Name()))
		if err != nil {
			continue
		}
		skills = append(skills, *skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills, nil
}

// ParseSkill reads SKILL.md inside skillFolderPath.
func ParseSkill(skillFolderPath string) (*Skill, error) {
	clean, err := ExpandPath(skillFolderPath)
	if err != nil {
		return nil, err
	}
	primary := filepath.Join(clean, "SKILL.md")
	path := primary
	if _, err := os.Lstat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		alt := filepath.Join(clean, "skill.md")
		if _, aerr := os.Lstat(alt); aerr != nil {
			if os.IsNotExist(aerr) {
				return nil, fmt.Errorf("%w: SKILL.md not found in %s", ErrNotFound, clean)
			}
			return nil, aerr
		}
		path = alt
	}
	return ParseSkillFile(path)
}

// ParseSkillFile reads one SKILL.md path.
func ParseSkillFile(skillFilePath string) (*Skill, error) {
	clean, err := ExpandPath(skillFilePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, clean)
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrInvalidPath, clean)
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, err
	}
	parsed, err := ParseFrontmatter(string(data))
	if err != nil {
		return nil, err
	}
	dirName := filepath.Base(filepath.Dir(clean))
	if err := ValidateFrontmatter(parsed.Frontmatter, dirName); err != nil {
		return nil, err
	}
	return &Skill{
		Name:        parsed.Frontmatter.Name,
		Description: parsed.Frontmatter.Description,
		Path:        filepath.Dir(clean),
		Content:     parsed.RawContent,
		Body:        parsed.Body,
		Extra:       parsed.Frontmatter.Extra,
	}, nil
}

// CreateSkill creates canonicalDir/name/SKILL.md.
// The name must be a valid skill name and must not case-collide with an
// existing folder.
func CreateSkill(canonicalDir, name, description, body string) (*Skill, error) {
	if err := ValidateSkillName(strings.TrimSpace(name)); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, ErrEmptyDescription
	}
	clean, err := ExpandPath(canonicalDir)
	if err != nil {
		return nil, err
	}
	if err := hermeticGuard(clean); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return nil, err
	}
	if err := caseCollision(clean, name); err != nil {
		return nil, err
	}
	dir, err := skillDir(clean, name)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrExists, name)
		}
		return nil, err
	}
	content, err := SerializeSkillMarkdown(Frontmatter{Name: name, Description: description}, body)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if err := writeAtomic(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &Skill{Name: name, Description: description, Path: dir, Content: content, Body: body}, nil
}

func caseCollision(dir, name string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == name {
			return fmt.Errorf("%w: %s", ErrExists, name)
		}
		if strings.EqualFold(entry.Name(), name) {
			return fmt.Errorf("%w: %q collides with existing %q", ErrInvalidName, name, entry.Name())
		}
	}
	return nil
}

// WriteSkill replaces SKILL.md in an existing skill folder.
// The write is atomic. The folder must already exist.
func WriteSkill(skillFolderPath, rawContent string) (*Skill, error) {
	clean, err := ExpandPath(skillFolderPath)
	if err != nil {
		return nil, err
	}
	if err := hermeticGuard(clean); err != nil {
		return nil, err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, clean)
		}
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s is not a skill directory", ErrInvalidPath, clean)
	}
	parsed, err := ParseFrontmatter(rawContent)
	if err != nil {
		return nil, err
	}
	if err := ValidateFrontmatter(parsed.Frontmatter, filepath.Base(clean)); err != nil {
		return nil, err
	}
	dest := filepath.Join(clean, "SKILL.md")
	if err := writeAtomic(dest, []byte(rawContent), 0o644); err != nil {
		return nil, err
	}
	return &Skill{
		Name:        parsed.Frontmatter.Name,
		Description: parsed.Frontmatter.Description,
		Path:        clean,
		Content:     rawContent,
		Body:        parsed.Body,
		Extra:       parsed.Frontmatter.Extra,
	}, nil
}

// RemoveSkill deletes a canonical skill directory this process can prove
// is inside canonicalDir. It does not follow symlinks.
func RemoveSkill(canonicalDir, skillName string) error {
	clean, err := ExpandPath(canonicalDir)
	if err != nil {
		return err
	}
	dir, err := skillDir(clean, skillName)
	if err != nil {
		return err
	}
	if err := hermeticGuard(dir); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, skillName)
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: refusing to remove symlink %s", ErrConflict, dir)
	}
	id, err := fileIdentity(dir)
	if err != nil {
		return err
	}
	return removeTreeNoFollow(dir, id)
}
