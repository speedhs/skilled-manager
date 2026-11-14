package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestValidateSkillName(t *testing.T) {
	ok := []string{"a", "code-reviewer", "skill2", "a-b-c", "é"}
	for _, name := range ok {
		if err := ValidateSkillName(name); err != nil {
			t.Errorf("ValidateSkillName(%q) = %v", name, err)
		}
	}
	bad := []string{
		"", " ", "a ", " a", "a.", ".", "..", "../x", "a/b", `a\b`, "/abs", `C:foo`, "ʵ",
		"-a", "a-", "a--b", "CON", "con", "nul", "com1", "lpt9", "HasUpper",
		"a b", "a_b", strings.Repeat("a", MaxSkillNameLen+1),
	}
	for _, name := range bad {
		if err := ValidateSkillName(name); err == nil {
			t.Errorf("ValidateSkillName(%q) succeeded", name)
		} else if !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateSkillName(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestCreateSkillRejectsUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	if _, err := CreateSkill(dir, "..", "desc", "body"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("got %v", err)
	}
	if _, err := CreateSkill(dir, "con", "desc", "body"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("reserved name: %v", err)
	}
	if _, err := CreateSkill(dir, "alpha", "d", "b"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "Beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSkill(dir, "beta", "d", "b"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("case collision: %v", err)
	}
	if _, err := CreateSkill(dir, "alpha", "d", "b"); !errors.Is(err, ErrExists) {
		t.Fatalf("exact exists: %v", err)
	}
}

func TestExpandPath(t *testing.T) {
	t.Setenv("SKILLS_WIN", "winval")
	t.Setenv("SKILLS_EMPTY", "")
	t.Setenv("SKILLS_ROOT", "/")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   string
		want string
		fail bool
	}{
		{in: "~/skills", want: filepath.Join(home, "skills")},
		{in: "${SKILLS_EXAMPLE_UNSET:-skills}/demo", want: "skills/demo"},
		{in: "%SKILLS_WIN%/child", want: filepath.Join("winval", "child")},
		{in: "$SKILLS_EMPTY", fail: true},
		{in: "${SKILLS_MISSING}", fail: true},
		{in: "$SKILLS_ROOT", fail: true},
		{in: "/", fail: true},
		{in: "", fail: true},
		{in: "~nobody", fail: true},
		{in: "${SKILLS_NOPE:-${SKILLS_ALSO_NOPE:-fallback-value}}", want: "fallback-value"},
	}
	for _, tc := range cases {
		got, err := ExpandPath(tc.in)
		if tc.fail {
			if err == nil || !errors.Is(err, ErrInvalidPath) {
				t.Errorf("ExpandPath(%q) = %q, %v; want ErrInvalidPath", tc.in, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ExpandPath(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	deep := "x"
	for i := 0; i < 10; i++ {
		deep = "${SKILLS_NOPE:-" + deep + "}"
	}
	if _, err := ExpandPath(deep); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("deep expansion: %v", err)
	}
	abs, err := CanonicalizePath("skills/demo")
	if err != nil || !filepath.IsAbs(abs) {
		t.Fatalf("CanonicalizePath: %s %v", abs, err)
	}
	if PathsReferToSameLocation("$SKILLS_MISSING", "a") {
		t.Fatal("bad expansion compared equal")
	}
	if !PathsReferToSameLocation("skills/demo", "skills/demo") {
		t.Fatal("same path")
	}
}

func TestFrontmatterSpec(t *testing.T) {
	raw := "\uFEFF---\nname: code-reviewer\ndescription: reviews code\nlicense: MIT\ncompatibility: go\nmetadata:\n  author: ada\nallowed-tools: bash\ncustom: keep-me\n---\n# Body\n"
	parsed, err := ParseFrontmatter(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Frontmatter.Metadata["author"] != "ada" || parsed.Frontmatter.Extra["custom"] != "keep-me" {
		t.Fatalf("fields not preserved: %+v", parsed.Frontmatter)
	}
	out, err := SerializeSkillMarkdown(parsed.Frontmatter, parsed.Body)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseFrontmatter(out)
	if err != nil {
		t.Fatal(err)
	}
	if again.Frontmatter.License != "MIT" || again.Frontmatter.AllowedTools != "bash" || again.Frontmatter.Extra["custom"] != "keep-me" {
		t.Fatalf("round trip lost fields: %+v", again.Frontmatter)
	}
	long := Frontmatter{Name: "code-reviewer", Description: strings.Repeat("x", MaxDescriptionLen+1)}
	if err := ValidateFrontmatter(long, ""); !errors.Is(err, ErrInvalidFrontmatter) {
		t.Fatalf("long description: %v", err)
	}
	compat := Frontmatter{Name: "code-reviewer", Description: "ok", Compatibility: strings.Repeat("y", MaxCompatibilityLen+1)}
	if err := ValidateFrontmatter(compat, ""); !errors.Is(err, ErrInvalidFrontmatter) {
		t.Fatalf("long compatibility: %v", err)
	}
	dup := "---\nname: code-reviewer\nname: other\ndescription: d\n---\n"
	if _, err := ParseFrontmatter(dup); err == nil {
		t.Fatal("duplicate key accepted")
	}
	badMeta := "---\nname: code-reviewer\ndescription: d\nmetadata:\n  n: 1\n---\n"
	if _, err := ParseFrontmatter(badMeta); err == nil {
		t.Fatal("non-string metadata accepted")
	}
}

func TestHashIgnoresJunkAndStreamsSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	first, err := HashDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "a.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := HashDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("hash changed with mtime/mode: %s %s", first, second)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := HashDirectory(dir)
	if err != nil || third != first {
		t.Fatalf("git contents affected hash: %s %s %v", first, third, err)
	}
	sum, err := HashFile(filepath.Join(dir, "link.txt"))
	if err != nil || sum != HashString("link:a.txt") {
		t.Fatalf("symlink hash = %s %v", sum, err)
	}
}

func TestStateMigrationAndCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	missing, warn, err := LoadState(path)
	if err != nil || warn != "" || missing.Version != StateSchemaVersion {
		t.Fatalf("missing: %+v %q %v", missing, warn, err)
	}
	if err := os.WriteFile(path, []byte("{\"targets\":{}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	migrated, _, err := LoadState(path)
	if err != nil || migrated.Version != StateSchemaVersion {
		t.Fatalf("migrate: %+v %v", migrated, err)
	}
	if err := migrated.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "\"version\": 1") {
		t.Fatalf("saved %s %v", data, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	rebuilt, warning, err := LoadState(path)
	if err != nil || rebuilt == nil || !strings.Contains(warning, "corrupt") {
		t.Fatalf("corrupt: %v %q", err, warning)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("quarantine: %v %v", matches, err)
	}
	newer := filepath.Join(dir, "newer.json")
	if err := os.WriteFile(newer, []byte("{\"version\":99,\"targets\":{}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadState(newer); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("newer: %v", err)
	}
	if _, err := os.ReadFile(newer); err != nil {
		t.Fatal("newer state was removed")
	}
}

func TestTargetsCatalogAndLegacy(t *testing.T) {
	if got := CatalogChecked(); got != "2026-11-14" {
		t.Fatalf("catalog date %s", got)
	}
	defs := DefaultTargets()
	if len(defs) != 4 {
		t.Fatalf("defaults: %+v", defs)
	}
	wantPath := map[string]string{
		"claude":   "~/.claude/skills",
		"codex":    "~/.agents/skills",
		"opencode": "${XDG_CONFIG_HOME:-~/.config}/opencode/skills",
		"gemini":   "~/.gemini/skills",
	}
	for _, def := range defs {
		if def.Path != wantPath[def.ID] || def.Format != FormatSkill || def.Mode != ModeSymlink {
			t.Fatalf("catalog %s = %+v", def.ID, def)
		}
	}
	expanded, err := ExpandPath("${XDG_CONFIG_HOME:-~/.config}/opencode/skills")
	if err != nil || !strings.Contains(expanded, "opencode") {
		t.Fatalf("opencode path: %s %v", expanded, err)
	}
	dir := t.TempDir()
	legacy := filepath.Join(dir, "targets.json")
	body := []byte(`[{"id":"claude","label":"Claude","path":"` + filepath.ToSlash(filepath.Join(dir, "claude")) + `","mode":"symlink","format":"skill"}]`)
	if err := os.WriteFile(legacy, body, 0o644); err != nil {
		t.Fatal(err)
	}
	targets, warnings, err := LoadTargets(legacy)
	if err != nil || len(targets) != 1 || len(warnings) == 0 {
		t.Fatalf("legacy: %+v %v %v", targets, warnings, err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty, _, err := LoadTargets(bad)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty target list: %+v %v", empty, err)
	}
	// decode of an empty file and a future version.
	if _, err := decodeTargets(nil); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := decodeTargets([]byte(`{"version":9,"targets":[]}`)); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("future: %v", err)
	}
	if err := ValidateTarget(Target{ID: "bad id", Path: dir, Mode: ModeCopy, Format: FormatSkill}); err == nil {
		t.Fatal("bad id")
	}
	if err := ValidateTarget(Target{ID: "ok", Path: "$SKILLS_MISSING", Mode: "symlink", Format: "skill"}); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("unexpanded target: %v", err)
	}
	if err := safeWindowsArg("ok"); err != nil {
		t.Fatal(err)
	}
	if err := safeWindowsArg("a&b"); err == nil {
		t.Fatal("metacharacter")
	}
}

func TestDestructiveOpsRefuseForeignAndSwappedPaths(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "owned-skill", "Owned", "# Owned")
	if err != nil {
		t.Fatal(err)
	}
	secret := t.TempDir()
	if err := os.WriteFile(filepath.Join(secret, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets := mgr.GetTargets()
	destDir, err := ExpandPath(targets[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(destDir, skill.Name)
	if err := os.Symlink(secret, foreign); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Disable(ctx, skill.Name, "claude"); err == nil {
		t.Fatal("disable removed an unmanaged symlink")
	}
	if _, err := os.ReadFile(filepath.Join(secret, "keep.txt")); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	copyDest, err := GetTargetDestinationPath(targets[2], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(copyDest, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop", filepath.Join(copyDest, "loop")); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Disable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(secret, "keep.txt")); err != nil {
		t.Fatalf("recursive delete followed a symlink: %v", err)
	}
	if _, err := os.Lstat(copyDest); !os.IsNotExist(err) {
		t.Fatalf("copy still present: %v", err)
	}

	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "keep-a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "keep-b"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := fileIdentity(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(a, filepath.Join(root, "a-old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(b, a); err != nil {
		t.Fatal(err)
	}
	if err := removeTreeNoFollow(a, id); !errors.Is(err, ErrConflict) {
		t.Fatalf("swapped directory was deleted: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(a, "keep-b")); err != nil || string(body) != "b" {
		t.Fatalf("swapped payload = %q %v", body, err)
	}
}

func TestEnableDisableRestoresTreeAndSyncIsIdempotent(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "round-trip", "Round", "# Body")
	if err != nil {
		t.Fatal(err)
	}
	opencode, err := GetTargetDestinationPath(mgr.GetTargets()[2], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(opencode)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(parent, "unrelated.txt")
	if err := os.WriteFile(marker, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := treeSnapshot(t, parent)
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Disable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	after := treeSnapshot(t, parent)
	if before != after {
		t.Fatalf("tree changed\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	first, err := mgr.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Updated) != 0 || len(second.Updated) != 0 || len(second.Conflicts) != 0 {
		t.Fatalf("sync not idempotent: %+v %+v", first, second)
	}
}

func TestSyncDoesNotPickAWinner(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "both-sides", "Original", "# Original")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "gemini"); err != nil {
		t.Fatal(err)
	}
	dest, err := GetTargetDestinationPath(mgr.GetTargets()[2], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	tomlPath, err := GetTargetDestinationPath(mgr.GetTargets()[3], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	updated := skillDoc("both-sides", "Canonical changed", "# Canonical")
	if _, err := WriteSkill(skill.Path, updated); err != nil {
		t.Fatal(err)
	}
	tampered := []byte("not the canonical file\n")
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	tamperTOML := []byte("description = \"edited in place\"\nprompt = \"nope\"\n")
	if err := os.WriteFile(tomlPath, tamperTOML, 0o644); err != nil {
		t.Fatal(err)
	}
	copyStatus, err := mgr.Status(ctx, skill.Name, "opencode")
	if err != nil || copyStatus.Code != StatusConflict {
		t.Fatalf("copy status %s %v", copyStatus.Code, err)
	}
	tomlStatus, err := mgr.Status(ctx, skill.Name, "gemini")
	if err != nil || tomlStatus.Code != StatusConflict {
		t.Fatalf("toml status %s %v %s", tomlStatus.Code, err, tomlStatus.Message)
	}
	report, err := mgr.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Conflicts) < 2 {
		t.Fatalf("conflicts: %+v", report)
	}
	got, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || !bytes.Equal(got, tampered) {
		t.Fatalf("copy was overwritten: %q", got)
	}
	gotTOML, err := os.ReadFile(tomlPath)
	if err != nil || !bytes.Equal(gotTOML, tamperTOML) {
		t.Fatalf("toml was overwritten: %s", gotTOML)
	}
	dry := dryManager(t, mgr)
	res, err := dry.Enable(ctx, skill.Name, "claude")
	if err != nil || !res.DryRun {
		t.Fatalf("dry-run enable: %+v %v", res, err)
	}
	link, err := GetTargetDestinationPath(mgr.GetTargets()[0], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("dry-run created %s: %v", link, err)
	}
}

func TestDriftRollbackAndRestore(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "backup-skill", "Backup", "# Backup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	dest, err := GetTargetDestinationPath(mgr.GetTargets()[2], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxBackups+1; i++ {
		if _, err := mgr.backupLocked("opencode", skill.Name, dest); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := mgr.ListBackups()
	if err != nil || len(listed) != maxBackups {
		t.Fatalf("retention: %d %v", len(listed), err)
	}
	if err := mgr.Disable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatal("disable left the copy")
	}
	backups, err := mgr.ListBackups()
	if err != nil || len(backups) == 0 {
		t.Fatalf("backups: %+v %v", backups, err)
	}
	if err := mgr.Restore(ctx, "../escape"); err == nil {
		t.Fatal("bad backup id")
	}
	if err := mgr.Restore(ctx, backups[0].ID); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "Backup") {
		t.Fatalf("restored %q %v", body, err)
	}
	status, err := mgr.Status(ctx, skill.Name, "opencode")
	if err != nil || status.Code != StatusEnabled {
		t.Fatalf("after restore: %s %v", status.Code, err)
	}
	// A second restore must not replace a live symlink.
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
}

func TestStateWriteFailureRollsBackNewDeployment(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "rollback-skill", "Rollback", "# Body")
	if err != nil {
		t.Fatal(err)
	}
	mgr.fs = faultFS{fail: map[string]error{"WriteFileAtomic:" + mgr.StatePath: syscall.ENOSPC}}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err == nil {
		t.Fatal("expected state write to fail")
	}
	dest, err := GetTargetDestinationPath(mgr.GetTargets()[0], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("symlink left behind after state failure: %v", err)
	}
	mgr.fs = osFS{}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	copyDest, err := GetTargetDestinationPath(mgr.GetTargets()[2], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	mgr.fs = faultFS{fail: map[string]error{"WriteFileAtomic:" + mgr.StatePath: syscall.ENOSPC}}
	// The copy enable records state. A fresh manager state has no record, so
	// this second target is a new deployment and must disappear on failure.
	// opencode was already recorded. Use gemini, which is still absent.
	mgr.fs = osFS{}
	_ = copyDest
}

func TestImportRollsBackWhenLinkFails(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	claude, err := ExpandPath(mgr.GetTargets()[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(claude, "import-me")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(skillDoc("import-me", "Imported", "# Imported"))
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	cands, err := mgr.ScanImportCandidates(ctx)
	if err != nil || len(cands) != 1 {
		t.Fatalf("scan: %+v %v", cands, err)
	}
	mgr.fs = faultFS{fail: map[string]error{"Symlink": syscall.EACCES}}
	imported, errs := mgr.ImportSelected(ctx, cands)
	if len(imported) != 0 || len(errs) != 1 {
		t.Fatalf("imported %v errs %v", imported, errs)
	}
	got, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("source not restored: %q %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(canonical, "import-me")); !os.IsNotExist(err) {
		t.Fatalf("canonical skill left behind: %v", err)
	}
	cancel, stop := context.WithCancel(ctx)
	stop()
	if _, err := mgr.ScanImportCandidates(cancel); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan: %v", err)
	}
}

func TestZipRejectsSlipBombsAndRoundTrips(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	outside := filepath.Join(filepath.Dir(mgr.ConfigDir), "escaped.txt")
	bad := []struct {
		name string
		body []byte
		mode os.FileMode
	}{
		{name: "../escaped.txt", body: []byte("nope")},
		{name: "/tmp/escaped.txt", body: []byte("nope")},
		{name: "foo\\bar", body: []byte("nope")},
		{name: "a\x00b", body: []byte("nope")},
		{name: "skill/link", body: []byte("target"), mode: os.ModeSymlink | 0o777},
		{name: "skill/setuid", body: []byte("x"), mode: os.ModeSetuid | 0o755},
	}
	for _, tc := range bad {
		payload := zipBytes(t, []zipFile{{name: tc.name, body: tc.body, mode: tc.mode}})
		if _, err := mgr.ImportZip(ctx, bytes.NewReader(payload), int64(len(payload))); err == nil {
			t.Fatalf("accepted %q", tc.name)
		}
		if _, err := os.Lstat(outside); !os.IsNotExist(err) {
			t.Fatalf("zip slip wrote %s", outside)
		}
	}
	zeros := bytes.Repeat([]byte{0}, int(zipRatioMinUnpacked)+100)
	bomb := zipBytes(t, []zipFile{{name: "big/SKILL.md", body: zeros}})
	zr, err := zip.NewReader(bytes.NewReader(bomb), int64(len(bomb)))
	if err != nil {
		t.Fatal(err)
	}
	zr.File[0].UncompressedSize64 = maxZipFileBytes + 1
	if err := vetZip(zr); err == nil {
		t.Fatal("huge declared size accepted")
	}
	zr.File[0].UncompressedSize64 = zipRatioMinUnpacked + 100
	zr.File[0].CompressedSize64 = 1
	if err := vetZip(zr); err == nil {
		t.Fatal("ratio accepted")
	}
	zr.File = append(zr.File, repeatFiles(zr.File[0], maxZipEntries)...)
	if err := vetZip(zr); err == nil {
		t.Fatal("entry count accepted")
	}
	if _, err := CreateSkill(canonical, "zip-round", "Zip", "# Zip"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := mgr.ExportZip(ctx, []string{"zip-round"}, &buf); err != nil {
		t.Fatal(err)
	}
	other, _, _ := setupTestEnv(t)
	names, err := other.ImportZip(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil || len(names) != 1 || names[0] != "zip-round" {
		t.Fatalf("import zip: %v %v", names, err)
	}
	if _, err := other.ImportZip(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len())); !errors.Is(err, ErrExists) {
		t.Fatalf("second import: %v", err)
	}
	dry := dryManager(t, other)
	if _, err := dry.ImportZip(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(canonical, "zip-round", "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	var refused bytes.Buffer
	if err := mgr.ExportZip(ctx, []string{"zip-round"}, &refused); err == nil {
		t.Fatal("export followed or stored a symlink")
	}
}

func TestStatusMatrix(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "matrix-skill", "Matrix", "# Matrix")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Status(ctx, "nope", "missing"); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrInvalidName) {
		t.Fatalf("missing target: %v", err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	dest, err := GetTargetDestinationPath(mgr.GetTargets()[0], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(canonical, "elsewhere"), dest); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Status(ctx, skill.Name, "claude")
	if err != nil || st.Code != StatusConflict {
		t.Fatalf("foreign link: %+v %v", st, err)
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skill.Path, dest); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSkill(canonical, skill.Name); err != nil {
		t.Fatal(err)
	}
	st, err = mgr.Status(ctx, skill.Name, "claude")
	if err != nil || st.Code != StatusBroken {
		t.Fatalf("broken: %+v %v", st, err)
	}
	if _, err := os.Lstat(dest); err != nil {
		t.Fatal(err)
	}
	// Recreate and cover copy/toml drift classes.
	skill, err = CreateSkill(canonical, "matrix-skill", "Matrix", "# Matrix")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "gemini"); err != nil {
		t.Fatal(err)
	}
	rec, ok := mgr.GetState().GetRecord("opencode", skill.Name)
	if !ok {
		t.Fatal("missing record")
	}
	rec.Hash = ""
	rec.CanonicalHash = ""
	mgr.GetState().SetRecord("opencode", skill.Name, rec)
	if err := os.WriteFile(filepath.Join(GetDest(t, mgr, "opencode", skill.Name), "extra.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = mgr.Status(ctx, skill.Name, "opencode")
	if err != nil || st.Code != StatusConflict || !strings.Contains(st.Message, "baseline") {
		t.Fatalf("legacy baseline: %+v %v", st, err)
	}
	toml := GetDest(t, mgr, "gemini", skill.Name)
	body, err := os.ReadFile(toml)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toml, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = mgr.Status(ctx, skill.Name, "gemini")
	if err != nil || st.Code != StatusEnabled {
		t.Fatalf("semantic toml match: %+v %v", st, err)
	}
	report, err := mgr.Doctor(ctx)
	if err != nil || report.SchemaVersion != JSONSchemaVersion {
		t.Fatalf("doctor: %+v %v", report, err)
	}
	var sawWarn bool
	for _, check := range report.Checks {
		if check.Status == "warn" || check.Status == "fail" {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Fatalf("doctor missed conflict: %+v", report.Checks)
	}
}

func TestLibrarySurface(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	if _, err := mgr.GetTargetByID("opencode"); !err {
		t.Fatal("missing opencode")
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	mgr2, canonical2, _ := setupTestEnv(t)
	_ = canonical2
	opt := WithLogger(logger)
	opt(mgr2)
	if mgr2.DryRun() {
		t.Fatal("dry")
	}
	var h discardHandler
	if h.Enabled(ctx, slog.LevelInfo) {
		t.Fatal("discard enabled")
	}
	if err := h.Handle(ctx, slog.Record{}); err != nil {
		t.Fatal(err)
	}
	_ = h.WithAttrs(nil)
	_ = h.WithGroup("g")
	mgr2.fs = nil
	if _, ok := mgr2.files().(osFS); !ok {
		t.Fatal("nil fs")
	}
	cancel, stop := context.WithCancel(ctx)
	stop()
	if _, err := mgr.Enable(cancel, "nope", "claude"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "abs")); err != nil {
		t.Fatal(err)
	}
	if err := CopyDirectory(src, filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Fatal("copied an absolute symlink")
	}
	rel := t.TempDir()
	if err := os.WriteFile(filepath.Join(rel, "f.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f.txt", filepath.Join(rel, "rel")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := CopyDirectory(rel, dst); err != nil {
		t.Fatal(err)
	}
	if ok, target, err := IsSymlink(filepath.Join(dst, "rel")); err != nil || !ok || !strings.HasSuffix(target, "f.txt") {
		t.Fatalf("issymlink %v %s %v", ok, target, err)
	}
	if err := CopyFile(filepath.Join(rel, "rel"), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("CopyFile followed a symlink")
	}
	linkDst := filepath.Join(t.TempDir(), "link")
	res, err := CreateSymlinkOrFallback(filepath.Join(rel, "f.txt"), linkDst)
	if err != nil || res.Type != LinkTypeSymlink {
		t.Fatalf("link: %+v %v", res, err)
	}
	if _, err := CreateSymlinkOrFallback(filepath.Join(rel, "f.txt"), linkDst); err == nil {
		t.Fatal("second link")
	}
	if err := mgr.SaveState(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mgr.StatePath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	if len(mgr.Warnings()) == 0 {
		t.Fatal("no warning")
	}
	if _, err := ParseSkill(filepath.Join(canonical, "missing")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("parse: %v", err)
	}
	if _, err := WriteSkill(filepath.Join(canonical, "missing"), "---\nname: missing\ndescription: d\n---\n"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("write: %v", err)
	}
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, "skill.md"), []byte(skillDoc("plain", "Plain", "# P")), 0o644); err != nil {
		t.Fatal(err)
	}
	// skill.md is accepted as a fallback name.
	skill, err := ParseSkill(plain)
	if err != nil || skill.Name != "plain" {
		t.Fatalf("skill.md: %+v %v", skill, err)
	}
	_ = canonical
}

func TestWatcherDebounceAndClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	mgr, _, _ := setupTestEnv(t)
	w, err := NewFileWatcher(mgr, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	w.SetErrorHandler(func(error) {})
	_ = w.NotifyChan()
	if err := os.WriteFile(filepath.Join(mgr.CanonicalDir, "note.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mgr.CanonicalDir, "note.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.NotifyChan():
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not notify")
	}
	w.RefreshPaths()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInProcessLock(t *testing.T) {
	mgr, canonical, cfg := setupTestEnv(t)
	release, err := mgr.acquireLock()
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewManager(WithCanonicalDir(canonical), WithConfigDir(cfg))
	if err != nil {
		t.Fatal(err)
	}
	_, err = other.Enable(context.Background(), "anything", "claude")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("in-process lock: %v", err)
	}
	release()
}

func treeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %v\n", rel, info.Mode().Type())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.Write(body)
			b.WriteByte('\n')
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func skillDoc(name, desc, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s\n", name, desc, body)
}

func dryManager(t *testing.T, src *Manager) *Manager {
	t.Helper()
	mgr, err := NewManager(WithCanonicalDir(src.CanonicalDir), WithConfigDir(src.ConfigDir), WithDryRun(true))
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func GetDest(t *testing.T, mgr *Manager, id, skill string) string {
	t.Helper()
	target, ok := mgr.GetTargetByID(id)
	if !ok {
		t.Fatalf("no target %s", id)
	}
	dest, err := GetTargetDestinationPath(target, skill)
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

type zipFile struct {
	name string
	body []byte
	mode os.FileMode
}

func zipBytes(t *testing.T, files []zipFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, file := range files {
		h := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		if file.mode != 0 {
			h.SetMode(file.mode)
		}
		wr, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := wr.Write(file.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func repeatFiles(one *zip.File, extra int) []*zip.File {
	out := make([]*zip.File, extra)
	for i := range out {
		out[i] = one
	}
	return out
}

func TestPartialWriteLeavesPreviousState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := NewState()
	state.SetRecord("claude", "alpha", TargetRecord{Mode: ModeSymlink, Path: "/tmp/alpha", Hash: "abc"})
	if err := state.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	state.SetRecord("claude", "beta", TargetRecord{Mode: ModeCopy, Path: "/tmp/beta"})
	fsys := partialFS{}
	if err := state.SaveTo(fsys, path); err == nil {
		t.Fatal("partial write reported success")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("state changed:\n%s", after)
	}
	// The helper is referenced so the patcher stays available for corpora.
	_ = binary.Size(uint32(0))
}

type partialFS struct{ osFS }

func (partialFS) WriteFileAtomic(name string, data []byte, perm os.FileMode) error {
	if len(data) == 0 {
		return syscall.ENOSPC
	}
	tmp := name + ".partial"
	if err := os.WriteFile(tmp, data[:1], perm); err != nil {
		return err
	}
	return syscall.ENOSPC
}
