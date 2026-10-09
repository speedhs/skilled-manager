package core

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter represents metadata stored at the top of a SKILL.md file.
type Frontmatter struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Extra       map[string]interface{} `yaml:",inline"`
}

// ParsedSkillContent holds the frontmatter and the remaining markdown body.
type ParsedSkillContent struct {
	Frontmatter Frontmatter
	Body        string
	RawContent  string
}

var (
	ErrMissingFrontmatter   = errors.New("file is missing YAML frontmatter delimiters (---)")
	ErrEmptySkillName       = errors.New("skill frontmatter 'name' cannot be empty")
	ErrEmptyDescription     = errors.New("skill frontmatter 'description' cannot be empty")
	ErrNameMismatchWithDir  = errors.New("skill frontmatter 'name' does not match the containing directory name")
)

// ParseFrontmatter extracts YAML frontmatter and markdown body from raw SKILL.md text.
func ParseFrontmatter(rawContent string) (*ParsedSkillContent, error) {
	trimmed := strings.TrimSpace(rawContent)
	if !strings.HasPrefix(trimmed, "---") {
		return nil, ErrMissingFrontmatter
	}

	// Find the end delimiter '---' or '...'
	contentAfterFirstDelim := trimmed[3:]
	// Must skip newline immediately after first '---'
	newlineIdx := strings.IndexAny(contentAfterFirstDelim, "\r\n")
	if newlineIdx == -1 {
		return nil, ErrMissingFrontmatter
	}

	rest := contentAfterFirstDelim[newlineIdx:]
	lines := strings.Split(rest, "\n")
	var yamlLines []string
	var bodyLines []string
	foundEnd := false

	for i, line := range lines {
		trimmedLine := strings.TrimRight(line, "\r")
		if !foundEnd && (trimmedLine == "---" || trimmedLine == "...") {
			foundEnd = true
			if i+1 < len(lines) {
				bodyLines = lines[i+1:]
			}
			break
		}
		if !foundEnd {
			yamlLines = append(yamlLines, line)
		}
	}

	if !foundEnd {
		return nil, fmt.Errorf("closing '---' frontmatter delimiter not found")
	}

	yamlContent := strings.Join(yamlLines, "\n")
	bodyContent := strings.Join(bodyLines, "\n")
	// Trim leading newline from body
	bodyContent = strings.TrimPrefix(bodyContent, "\r\n")
	bodyContent = strings.TrimPrefix(bodyContent, "\n")

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	return &ParsedSkillContent{
		Frontmatter: fm,
		Body:        bodyContent,
		RawContent:  rawContent,
	}, nil
}

// ValidateFrontmatter validates the parsed frontmatter.
// If expectedFolderName is provided, it verifies fm.Name matches it.
func ValidateFrontmatter(fm Frontmatter, expectedFolderName string) error {
	trimmedName := strings.TrimSpace(fm.Name)
	if trimmedName == "" {
		return ErrEmptySkillName
	}

	trimmedDesc := strings.TrimSpace(fm.Description)
	if trimmedDesc == "" {
		return ErrEmptyDescription
	}

	if expectedFolderName != "" {
		expectedClean := strings.TrimSpace(expectedFolderName)
		if expectedClean != "" && !strings.EqualFold(trimmedName, expectedClean) && trimmedName != expectedClean {
			return fmt.Errorf("%w (frontmatter name: %q, folder: %q)", ErrNameMismatchWithDir, trimmedName, expectedClean)
		}
	}

	return nil
}

// SerializeSkillMarkdown formats frontmatter and markdown body into standard SKILL.md format.
func SerializeSkillMarkdown(fm Frontmatter, body string) (string, error) {
	if strings.TrimSpace(fm.Name) == "" {
		return "", ErrEmptySkillName
	}
	if strings.TrimSpace(fm.Description) == "" {
		return "", ErrEmptyDescription
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm); err != nil {
		return "", fmt.Errorf("failed to encode YAML frontmatter: %w", err)
	}
	_ = encoder.Close()

	buf.WriteString("---\n")
	if body != "" {
		// Ensure single newline before body if not already present
		if !strings.HasPrefix(body, "\n") && !strings.HasPrefix(body, "\r\n") {
			buf.WriteString("\n")
		}
		buf.WriteString(strings.TrimRight(body, "\r\n") + "\n")
	}

	return buf.String(), nil
}
