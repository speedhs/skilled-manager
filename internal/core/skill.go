package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill represents a loaded skill from the canonical store.
type Skill struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Path        string                 `json:"path"`
	Content     string                 `json:"content"`
	Body        string                 `json:"body"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// ListSkills scans canonicalDir for skill subfolders and returns parsed skills sorted by name.
func ListSkills(canonicalDir string) ([]Skill, error) {
	cleanDir := ExpandPath(canonicalDir)
	if _, err := os.Stat(cleanDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cleanDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create canonical skills directory %s: %w", cleanDir, err)
		}
		return []Skill{}, nil
	}

	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read canonical skills directory %s: %w", cleanDir, err)
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		folderPath := filepath.Join(cleanDir, entry.Name())
		skill, err := ParseSkill(folderPath)
		if err != nil {
			// Skip or include with partial info
			continue
		}
		skills = append(skills, *skill)
	}

	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})

	return skills, nil
}

// ParseSkill parses the SKILL.md file inside a skill folder.
func ParseSkill(skillFolderPath string) (*Skill, error) {
	cleanPath := ExpandPath(skillFolderPath)
	skillFilePath := filepath.Join(cleanPath, "SKILL.md")

	if _, err := os.Stat(skillFilePath); os.IsNotExist(err) {
		// Try lowercase skill.md as fallback
		skillFilePath = filepath.Join(cleanPath, "skill.md")
		if _, err := os.Stat(skillFilePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("SKILL.md not found in directory: %s", cleanPath)
		}
	}

	return ParseSkillFile(skillFilePath)
}

// ParseSkillFile parses a specific SKILL.md file.
func ParseSkillFile(skillFilePath string) (*Skill, error) {
	cleanPath := ExpandPath(skillFilePath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file %s: %w", cleanPath, err)
	}

	parsed, err := ParseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter in %s: %w", cleanPath, err)
	}

	dirName := filepath.Base(filepath.Dir(cleanPath))
	if err := ValidateFrontmatter(parsed.Frontmatter, dirName); err != nil {
		return nil, fmt.Errorf("invalid skill metadata in %s: %w", cleanPath, err)
	}

	return &Skill{
		Name:        parsed.Frontmatter.Name,
		Description: parsed.Frontmatter.Description,
		Path:        filepath.Dir(cleanPath),
		Content:     parsed.RawContent,
		Body:        parsed.Body,
		Extra:       parsed.Frontmatter.Extra,
	}, nil
}

// CreateSkill creates a new skill directory in canonicalDir with SKILL.md.
func CreateSkill(canonicalDir, name, description, body string) (*Skill, error) {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return nil, ErrEmptySkillName
	}
	cleanDesc := strings.TrimSpace(description)
	if cleanDesc == "" {
		return nil, ErrEmptyDescription
	}

	// Ensure canonical directory exists
	cleanDir := ExpandPath(canonicalDir)
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create canonical dir %s: %w", cleanDir, err)
	}

	skillFolderPath := filepath.Join(cleanDir, cleanName)
	if _, err := os.Stat(skillFolderPath); err == nil {
		return nil, fmt.Errorf("skill %q already exists in canonical store at %s", cleanName, skillFolderPath)
	}

	if err := os.MkdirAll(skillFolderPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create skill directory %s: %w", skillFolderPath, err)
	}

	fm := Frontmatter{
		Name:        cleanName,
		Description: cleanDesc,
	}

	content, err := SerializeSkillMarkdown(fm, body)
	if err != nil {
		return nil, err
	}

	skillFilePath := filepath.Join(skillFolderPath, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write SKILL.md in %s: %w", skillFolderPath, err)
	}

	return &Skill{
		Name:        cleanName,
		Description: cleanDesc,
		Path:        skillFolderPath,
		Content:     content,
		Body:        body,
	}, nil
}

// WriteSkill updates the SKILL.md in an existing skill folder with validation.
func WriteSkill(skillFolderPath, rawContent string) (*Skill, error) {
	cleanPath := ExpandPath(skillFolderPath)
	dirName := filepath.Base(cleanPath)

	parsed, err := ParseFrontmatter(rawContent)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if err := ValidateFrontmatter(parsed.Frontmatter, dirName); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", cleanPath, err)
	}

	skillFilePath := filepath.Join(cleanPath, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(rawContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", skillFilePath, err)
	}

	return &Skill{
		Name:        parsed.Frontmatter.Name,
		Description: parsed.Frontmatter.Description,
		Path:        cleanPath,
		Content:     rawContent,
		Body:        parsed.Body,
		Extra:       parsed.Frontmatter.Extra,
	}, nil
}

// RemoveSkill removes a skill's canonical folder.
func RemoveSkill(canonicalDir, skillName string) error {
	cleanDir := ExpandPath(canonicalDir)
	skillPath := filepath.Join(cleanDir, skillName)
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found in %s", skillName, cleanDir)
	}
	return os.RemoveAll(skillPath)
}
