package core

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// GeminiCommand represents the structure of a Gemini CLI command file (.toml).
type GeminiCommand struct {
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`
}

// SkillToGeminiTOML converts a Skill into Gemini command TOML format.
func SkillToGeminiTOML(skill Skill) (string, error) {
	cmd := GeminiCommand{
		Description: skill.Description,
		Prompt:      skill.Body,
	}

	data, err := toml.Marshal(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to encode Gemini TOML: %w", err)
	}

	return string(data), nil
}

// GeminiTOMLToSkill converts Gemini TOML content into a Skill representation.
func GeminiTOMLToSkill(skillName, tomlContent string) (*Skill, error) {
	var cmd GeminiCommand
	if err := toml.Unmarshal([]byte(tomlContent), &cmd); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini TOML for skill %q: %w", skillName, err)
	}

	fm := Frontmatter{
		Name:        skillName,
		Description: cmd.Description,
	}

	markdown, err := SerializeSkillMarkdown(fm, cmd.Prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize markdown from Gemini TOML: %w", err)
	}

	return &Skill{
		Name:        skillName,
		Description: cmd.Description,
		Body:        cmd.Prompt,
		Content:     markdown,
	}, nil
}

// IsGeminiTOMLMatch compares actual TOML content in target with canonical skill
func IsGeminiTOMLMatch(canonical Skill, actualTOMLContent string) bool {
	var cmd GeminiCommand
	if err := toml.Unmarshal([]byte(actualTOMLContent), &cmd); err != nil {
		return false
	}

	return strings.TrimSpace(cmd.Description) == strings.TrimSpace(canonical.Description) &&
		strings.TrimSpace(cmd.Prompt) == strings.TrimSpace(canonical.Body)
}
