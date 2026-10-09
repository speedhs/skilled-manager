package core

import (
	"strings"
	"testing"
)

func TestGeminiTOMLConversion(t *testing.T) {
	skill := Skill{
		Name:        "summarizer",
		Description: "Summarizes text and documents into bullet points",
		Body: `# Summarizer
Please summarize the following text:
- Focus on key takeaways
- Keep it concise
`,
	}

	tomlStr, err := SkillToGeminiTOML(skill)
	if err != nil {
		t.Fatalf("failed to convert skill to Gemini TOML: %v", err)
	}

	if !strings.Contains(tomlStr, "description = 'Summarizes text and documents into bullet points'") &&
		!strings.Contains(tomlStr, `description = "Summarizes text and documents into bullet points"`) {
		t.Errorf("toml output missing description: %s", tomlStr)
	}

	if !strings.Contains(tomlStr, "Focus on key takeaways") {
		t.Errorf("toml output missing prompt content: %s", tomlStr)
	}

	// Round-trip conversion
	reconstructed, err := GeminiTOMLToSkill("summarizer", tomlStr)
	if err != nil {
		t.Fatalf("failed to convert Gemini TOML back to skill: %v", err)
	}

	if reconstructed.Name != "summarizer" {
		t.Errorf("expected reconstructed name 'summarizer', got %q", reconstructed.Name)
	}
	if reconstructed.Description != skill.Description {
		t.Errorf("expected description %q, got %q", skill.Description, reconstructed.Description)
	}
	if strings.TrimSpace(reconstructed.Body) != strings.TrimSpace(skill.Body) {
		t.Errorf("expected body %q, got %q", skill.Body, reconstructed.Body)
	}

	// Test Match helper
	if !IsGeminiTOMLMatch(skill, tomlStr) {
		t.Errorf("expected IsGeminiTOMLMatch to return true")
	}

	// Test Match failure on changed content
	diffSkill := skill
	diffSkill.Description = "Different description"
	if IsGeminiTOMLMatch(diffSkill, tomlStr) {
		t.Errorf("expected IsGeminiTOMLMatch to return false for changed description")
	}
}
