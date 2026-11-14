package core

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

const (
	// MaxDescriptionLen is the Agent Skills limit for description.
	MaxDescriptionLen = 1024
	// MaxCompatibilityLen is the Agent Skills limit for compatibility.
	MaxCompatibilityLen = 500
)

// Frontmatter is the metadata block at the top of a SKILL.md file.
// Extra holds unknown keys so a newer field round-trips unchanged.
type Frontmatter struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  string
	Extra         map[string]any
}

// ParsedSkillContent is a SKILL.md split into metadata and body.
type ParsedSkillContent struct {
	Frontmatter Frontmatter
	Body        string
	RawContent  string
}

var (
	// ErrMissingFrontmatter means the file does not start with a --- block.
	ErrMissingFrontmatter = fmt.Errorf("%w: missing opening ---", ErrInvalidFrontmatter)

	// ErrEmptySkillName means name is missing or whitespace.
	ErrEmptySkillName = fmt.Errorf("%w: name is empty", ErrInvalidName)

	// ErrEmptyDescription means description is missing or whitespace.
	ErrEmptyDescription = fmt.Errorf("%w: description is empty", ErrInvalidFrontmatter)

	// ErrNameMismatchWithDir means name is not the directory name.
	ErrNameMismatchWithDir = fmt.Errorf("%w: name does not match directory", ErrInvalidName)
)

// knownFrontmatterFields are the Agent Skills keys. Anything else is Extra.
var knownFrontmatterFields = map[string]struct{}{
	"name": {}, "description": {}, "license": {},
	"compatibility": {}, "metadata": {}, "allowed-tools": {},
}

// ParseFrontmatter splits raw SKILL.md text into YAML metadata and body.
// Unknown keys are kept. Duplicate keys are rejected.
func ParseFrontmatter(rawContent string) (*ParsedSkillContent, error) {
	raw := strings.TrimPrefix(rawContent, "\uFEFF")
	yamlText, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}
	fm, err := decodeFrontmatter(yamlText)
	if err != nil {
		return nil, err
	}
	return &ParsedSkillContent{Frontmatter: fm, Body: body, RawContent: rawContent}, nil
}

func splitFrontmatter(raw string) (string, string, error) {
	first, rest, ok := cutLine(raw)
	if !ok || first != "---" {
		return "", "", ErrMissingFrontmatter
	}
	var yamlLines []string
	for {
		line, next, ok := cutLine(rest)
		if !ok {
			return "", "", fmt.Errorf("%w: closing --- not found", ErrInvalidFrontmatter)
		}
		rest = next
		if line == "---" || line == "..." {
			body := rest
			body = strings.TrimPrefix(body, "\n")
			return strings.Join(yamlLines, "\n"), body, nil
		}
		yamlLines = append(yamlLines, line)
	}
}

func cutLine(s string) (string, string, bool) {
	if s == "" {
		return "", "", false
	}
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return strings.TrimRight(s, "\r"), "", true
	}
	return strings.TrimRight(s[:i], "\r"), s[i+1:], true
}

func decodeFrontmatter(yamlText string) (Frontmatter, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return Frontmatter{}, fmt.Errorf("%w: %v", ErrInvalidFrontmatter, err)
	}
	if len(doc.Content) == 0 {
		return Frontmatter{}, fmt.Errorf("%w: frontmatter is empty", ErrInvalidFrontmatter)
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return Frontmatter{}, fmt.Errorf("%w: frontmatter must be a map", ErrInvalidFrontmatter)
	}
	fields := map[string]*yaml.Node{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			return Frontmatter{}, fmt.Errorf("%w: field names must be strings", ErrInvalidFrontmatter)
		}
		key := keyNode.Value
		if _, dup := fields[key]; dup {
			return Frontmatter{}, fmt.Errorf("%w: duplicate field %q", ErrInvalidFrontmatter, key)
		}
		fields[key] = mapping.Content[i+1]
	}
	var fm Frontmatter
	fm.Extra = map[string]any{}
	var err error
	if fm.Name, err = scalarField(fields, "name"); err != nil {
		return Frontmatter{}, err
	}
	if fm.Description, err = scalarField(fields, "description"); err != nil {
		return Frontmatter{}, err
	}
	if fm.License, err = scalarField(fields, "license"); err != nil {
		return Frontmatter{}, err
	}
	if fm.Compatibility, err = scalarField(fields, "compatibility"); err != nil {
		return Frontmatter{}, err
	}
	if fm.AllowedTools, err = scalarField(fields, "allowed-tools"); err != nil {
		return Frontmatter{}, err
	}
	if node, ok := fields["metadata"]; ok && node.Tag != "!!null" && node.Kind != 0 {
		meta, err := decodeMetadata(node)
		if err != nil {
			return Frontmatter{}, err
		}
		fm.Metadata = meta
	}
	for key, node := range fields {
		if _, known := knownFrontmatterFields[key]; known {
			continue
		}
		var v any
		if err := node.Decode(&v); err != nil {
			return Frontmatter{}, fmt.Errorf("%w: field %s: %v", ErrInvalidFrontmatter, key, err)
		}
		fm.Extra[key] = v
	}
	if len(fm.Extra) == 0 {
		fm.Extra = nil
	}
	return fm, nil
}

func scalarField(fields map[string]*yaml.Node, key string) (string, error) {
	node, ok := fields[key]
	if !ok || node.Tag == "!!null" {
		return "", nil
	}
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%w: %s must be a string", ErrInvalidFrontmatter, key)
	}
	return node.Value, nil
}

func decodeMetadata(node *yaml.Node) (map[string]string, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: metadata must be a map of strings", ErrInvalidFrontmatter)
	}
	out := map[string]string{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode || (v.Tag != "" && v.Tag != "!!str") {
			return nil, fmt.Errorf("%w: metadata values must be strings", ErrInvalidFrontmatter)
		}
		if _, dup := out[k.Value]; dup {
			return nil, fmt.Errorf("%w: duplicate metadata key %q", ErrInvalidFrontmatter, k.Value)
		}
		out[k.Value] = v.Value
	}
	return out, nil
}

// ValidateFrontmatter checks fm against the Agent Skills spec.
// expectedFolderName, when not empty, must equal name exactly.
func ValidateFrontmatter(fm Frontmatter, expectedFolderName string) error {
	if strings.TrimSpace(fm.Name) == "" {
		return ErrEmptySkillName
	}
	if err := ValidateSkillName(fm.Name); err != nil {
		return err
	}
	if strings.TrimSpace(fm.Description) == "" {
		return ErrEmptyDescription
	}
	if utf8.RuneCountInString(fm.Description) > MaxDescriptionLen {
		return fmt.Errorf("%w: description exceeds %d characters", ErrInvalidFrontmatter, MaxDescriptionLen)
	}
	if utf8.RuneCountInString(fm.Compatibility) > MaxCompatibilityLen {
		return fmt.Errorf("%w: compatibility exceeds %d characters", ErrInvalidFrontmatter, MaxCompatibilityLen)
	}
	if expectedFolderName != "" {
		folder := norm.NFKC.String(strings.TrimSpace(expectedFolderName))
		if fm.Name != folder {
			return fmt.Errorf("%w (frontmatter name: %q, folder: %q)", ErrNameMismatchWithDir, fm.Name, folder)
		}
	}
	return nil
}

// SerializeSkillMarkdown writes fm and body as a SKILL.md document.
// Unknown Extra keys are preserved. Validation errors are returned.
func SerializeSkillMarkdown(fm Frontmatter, body string) (string, error) {
	if err := ValidateFrontmatter(fm, ""); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\n")
	writeScalar(&b, "name", fm.Name)
	writeScalar(&b, "description", fm.Description)
	if fm.License != "" {
		writeScalar(&b, "license", fm.License)
	}
	if fm.Compatibility != "" {
		writeScalar(&b, "compatibility", fm.Compatibility)
	}
	if len(fm.Metadata) > 0 {
		chunk, err := yaml.Marshal(map[string]any{"metadata": fm.Metadata})
		if err != nil {
			return "", err
		}
		b.Write(chunk)
	}
	if fm.AllowedTools != "" {
		writeScalar(&b, "allowed-tools", fm.AllowedTools)
	}
	if len(fm.Extra) > 0 {
		keys := make([]string, 0, len(fm.Extra))
		for k := range fm.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			chunk, err := yaml.Marshal(map[string]any{k: fm.Extra[k]})
			if err != nil {
				return "", err
			}
			b.Write(chunk)
		}
	}
	b.WriteString("---\n")
	if body != "" {
		if !strings.HasPrefix(body, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimRight(body, "\r\n"))
		b.WriteString("\n")
	}
	return b.String(), nil
}

func writeScalar(b *strings.Builder, key, value string) {
	chunk, err := yaml.Marshal(map[string]string{key: value})
	if err != nil {
		fmt.Fprintf(b, "%s: %q\n", key, value)
		return
	}
	b.Write(chunk)
}
