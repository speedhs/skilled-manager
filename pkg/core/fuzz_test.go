package core

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func FuzzParseFrontmatter(f *testing.F) {
	f.Add("---\nname: code-reviewer\ndescription: reviews code\n---\n# Body\n")
	f.Add("no frontmatter")
	f.Add("---\nname: a\nname: b\ndescription: d\n---\n")
	f.Fuzz(func(t *testing.T, raw string) {
		parsed, err := ParseFrontmatter(raw)
		if err != nil || parsed == nil {
			return
		}
		out, err := SerializeSkillMarkdown(parsed.Frontmatter, parsed.Body)
		if err != nil {
			return
		}
		again, err := ParseFrontmatter(out)
		if err != nil {
			t.Fatalf("round trip: %v\n%s", err, out)
		}
		if again.Frontmatter.Name != parsed.Frontmatter.Name {
			t.Fatalf("name %q became %q", parsed.Frontmatter.Name, again.Frontmatter.Name)
		}
	})
}

func FuzzExpandPath(f *testing.F) {
	f.Add("~/.claude/skills")
	f.Add("$HOME/skills")
	f.Add("${MISSING}")
	f.Add("${MISSING:-fallback}")
	f.Add("%HOME%")
	f.Add("/")
	f.Fuzz(func(t *testing.T, in string) {
		got, err := ExpandPath(in)
		if err != nil {
			return
		}
		if got == "" || got == "/" || got == `\` {
			t.Fatalf("ExpandPath(%q) = %q", in, got)
		}
	})
}

func FuzzValidateSkillName(f *testing.F) {
	f.Add("code-reviewer")
	f.Add("..")
	f.Add("con")
	f.Add("COM1")
	f.Fuzz(func(t *testing.T, name string) {
		err := ValidateSkillName(name)
		if err != nil {
			return
		}
		if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
			t.Fatalf("accepted %q", name)
		}
		if name != strings.ToLower(name) || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") || norm.NFKC.String(name) != name {
			t.Fatalf("accepted %q", name)
		}
		for _, r := range name {
			if r == '-' || unicode.IsDigit(r) || (unicode.IsLetter(r) && unicode.IsLower(r)) {
				continue
			}
			t.Fatalf("accepted %q", name)
		}
	})
}

func FuzzImportZip(f *testing.F) {
	f.Add([]byte("PK"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		mgr, err := NewManager(WithCanonicalDir(filepath.Join(dir, "canon")), WithConfigDir(filepath.Join(dir, "cfg")))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = mgr.ImportZip(context.Background(), bytes.NewReader(data), int64(len(data)))
		entries, err := os.ReadDir(mgr.CanonicalDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if err := ValidateSkillName(entry.Name()); err != nil {
				t.Fatalf("import created %q", entry.Name())
			}
		}
	})
}

func FuzzGeminiTOML(f *testing.F) {
	f.Add("description = \"hi\"\nprompt = \"body\"\n")
	f.Add("not toml")
	f.Fuzz(func(t *testing.T, text string) {
		skill, err := GeminiTOMLToSkill("summarizer", text)
		if err != nil || skill == nil {
			return
		}
		out, err := SkillToGeminiTOML(*skill)
		if err != nil {
			t.Fatal(err)
		}
		if !IsGeminiTOMLMatch(*skill, out) {
			t.Fatalf("toml did not match itself: %s", out)
		}
	})
}
