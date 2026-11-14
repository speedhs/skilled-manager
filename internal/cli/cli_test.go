package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	orig, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	fake, err := os.MkdirTemp("", "skills-cli-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("SKILLS_ORIGINAL_HOME", orig)
	_ = os.Setenv("HOME", fake)
	_ = os.Setenv("USERPROFILE", fake)
	_ = os.Setenv("SKILLS_TEST_MODE", "1")
	_ = os.Unsetenv("XDG_CONFIG_HOME")
	code := m.Run()
	_ = os.RemoveAll(fake)
	os.Exit(code)
}

func TestUsageAndVersion(t *testing.T) {
	if code, _, _ := run(t, nil); code != ExitUsage {
		t.Fatalf("no args: %d", code)
	}
	code, stdout, stderr := run(t, []string{"--help"})
	if code != ExitOK || !strings.Contains(stdout, "enable <skill>") {
		t.Fatalf("help: %d %s %s", code, stdout, stderr)
	}
	code, stdout, _ = run(t, []string{"version"})
	if code != ExitOK || !strings.Contains(stdout, "skills dev") {
		t.Fatalf("version: %s", stdout)
	}
	code, stdout, _ = run(t, []string{"--json", "version"})
	var doc struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || doc.SchemaVersion != 1 || code != ExitOK {
		t.Fatalf("json version: %d %s %v", code, stdout, err)
	}
	code, stdout, _ = run(t, []string{"completion", "bash"})
	if code != ExitOK || !strings.Contains(stdout, "enable") {
		t.Fatalf("completion: %s", stdout)
	}
	for _, shell := range []string{"zsh", "fish", "pwsh"} {
		if code, _, stderr := run(t, []string{"completion", shell}); code != ExitOK {
			t.Fatalf("%s: %d %s", shell, code, stderr)
		}
	}
	if code, stdout, _ := run(t, []string{"man"}); code != ExitOK || !strings.Contains(stdout, ".TH SKILLS") {
		t.Fatal(stdout)
	}
	if code, _, stderr := run(t, []string{"nope"}); code != ExitUsage || !strings.Contains(stderr, "unknown") {
		t.Fatalf("unknown: %d %s", code, stderr)
	}
	t.Setenv("NO_COLOR", "1")
	_, stdout, stderr = run(t, []string{"version"})
	if strings.Contains(stdout+stderr, "\x1b") {
		t.Fatal("emitted color")
	}
}

func TestNewListEnableDisable(t *testing.T) {
	canon, cfg := t.TempDir(), t.TempDir()
	base := []string{"--canonical", canon, "--config", cfg}
	code, _, stderr := run(t, append(base, "new", "demo-skill", "A demo"))
	if code != ExitOK {
		t.Fatal(stderr)
	}
	code, stdout, stderr := run(t, append(base, "--json", "list"))
	if code != ExitOK {
		t.Fatal(stderr)
	}
	var listed struct {
		SchemaVersion int `json:"schema_version"`
		Skills        []struct {
			Name string `json:"name"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil || listed.SchemaVersion != 1 || len(listed.Skills) != 1 {
		t.Fatalf("list: %s %v", stdout, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".claude", "skills", "demo-skill")
	code, _, stderr = run(t, append(base, "--dry-run", "enable", "demo-skill", "claude"))
	if code != ExitOK {
		t.Fatal(stderr)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("dry-run created %s", dest)
	}
	code, _, stderr = run(t, append(base, "enable", "demo-skill", "claude"))
	if code != ExitOK {
		t.Fatal(stderr)
	}
	if _, err := os.Lstat(dest); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run(t, append(base, "disable", "demo-skill", "claude"))
	if code != ExitUsage {
		t.Fatalf("disable without --yes: %d %s", code, stderr)
	}
	if _, err := os.Lstat(dest); err != nil {
		t.Fatal("disable removed the link without confirmation")
	}
	code, _, stderr = run(t, append(base, "--yes", "disable", "demo-skill", "claude"))
	if code != ExitOK {
		t.Fatal(stderr)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatal("link remained")
	}
}

func TestConflictAndPartial(t *testing.T) {
	canon, cfg := t.TempDir(), t.TempDir()
	base := []string{"--canonical", canon, "--config", cfg}
	if code, _, stderr := run(t, append(base, "new", "demo-skill", "A demo")); code != ExitOK {
		t.Fatal(stderr)
	}
	home, _ := os.UserHomeDir()
	foreign := filepath.Join(home, ".claude", "skills", "demo-skill")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "mine.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, append(base, "enable", "demo-skill", "claude"))
	if code != ExitConflict {
		t.Fatalf("conflict exit %d %s", code, stderr)
	}
	if _, err := os.ReadFile(filepath.Join(foreign, "mine.txt")); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run(t, append(base, "enable", "demo-skill"))
	if code != ExitPartial {
		t.Fatalf("partial exit %d %s", code, stderr)
	}
	code, stdout, stderr := run(t, append(base, "--json", "doctor"))
	if code != ExitOK && code != ExitError {
		t.Fatal(stderr)
	}
	var report struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.SchemaVersion != 1 {
		t.Fatalf("doctor: %s %v", stdout, err)
	}
	code, stdout, stderr = run(t, append(base, "--json", "sync"))
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.SchemaVersion != 1 {
		t.Fatalf("sync: %d %s %s %v", code, stdout, stderr, err)
	}
	code, stdout, stderr = run(t, append(base, "--json", "targets"))
	if code != ExitOK || !strings.Contains(stdout, "expanded") {
		t.Fatalf("targets: %s %s", stdout, stderr)
	}
	code, stdout, stderr = run(t, append(base, "--json", "import", "--scan"))
	if code != ExitOK || !strings.Contains(stdout, "candidates") {
		t.Fatalf("scan: %s %s", stdout, stderr)
	}
}

func run(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
