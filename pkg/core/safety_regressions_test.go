package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDisableLeavesEditedAndReplacedCopies(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	if _, err := CreateSkill(canonical, "edited", "Edited", "# Edited"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, "edited", "opencode"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", "edited")
	notes := filepath.Join(dest, "my-notes.txt")
	if err := os.WriteFile(notes, []byte("user work"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Disable(ctx, "edited", "opencode"); !errors.Is(err, ErrConflict) {
		t.Fatalf("disable of an edited copy: %v", err)
	}
	body, err := os.ReadFile(notes)
	if err != nil || string(body) != "user work" {
		t.Fatalf("edited copy = %q %v", body, err)
	}

	if _, err := CreateSkill(canonical, "replaced", "Replaced", "# Replaced"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, "replaced", "opencode"); err != nil {
		t.Fatal(err)
	}
	replaced := GetDest(t, mgr, "opencode", "replaced")
	if err := os.RemoveAll(replaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(replaced, 0o755); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(replaced, "SKILL.md")
	if err := os.WriteFile(own, []byte("---\nname: replaced\ndescription: the user's own skill\n---\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Disable(ctx, "replaced", "opencode"); !errors.Is(err, ErrConflict) {
		t.Fatalf("disable after the directory was recreated: %v", err)
	}
	if _, err := os.ReadFile(own); err != nil {
		t.Fatalf("recreated directory was removed: %v", err)
	}
}

func TestMutationReloadsStateFromDisk(t *testing.T) {
	first, canonical, config := setupTestEnv(t)
	ctx := context.Background()
	for _, name := range []string{"skill-a", "skill-b"} {
		if _, err := CreateSkill(canonical, name, "Desc", "# Body"); err != nil {
			t.Fatal(err)
		}
	}
	second, err := NewManager(WithCanonicalDir(canonical), WithConfigDir(config))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Enable(ctx, "skill-a", "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Enable(ctx, "skill-b", "opencode"); err != nil {
		t.Fatal(err)
	}
	third, err := NewManager(WithCanonicalDir(canonical), WithConfigDir(config))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skill-a", "skill-b"} {
		st, err := third.Status(ctx, name, "opencode")
		if err != nil || st.Code != StatusEnabled {
			t.Fatalf("%s after two processes wrote state: %+v %v", name, st, err)
		}
	}
}

func TestDisableStopsWhenBackupFails(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	if _, err := CreateSkill(canonical, "kept", "Kept", "# Kept"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, "kept", "opencode"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", "kept")
	mgr.fs = failOpFS{op: "CopyFile", err: syscall.EIO}
	if err := mgr.Disable(ctx, "kept", "opencode"); err == nil {
		t.Fatal("disable continued after the backup failed")
	}
	body, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "Kept") {
		t.Fatalf("deployment removed when backup failed: %q %v", body, err)
	}
}

func TestBackupRefusesToDropTheCopyJustMade(t *testing.T) {
	mgr, _, _ := setupTestEnv(t)
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxBackupBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.backupLocked("opencode", "big-skill", dir); err == nil {
		t.Fatal("accepted a backup larger than the retention limit")
	}
	listed, err := mgr.ListBackups()
	if err != nil || len(listed) != 0 {
		t.Fatalf("backups after a refused copy: %+v %v", listed, err)
	}

	root := filepath.Join(mgr.ConfigDir, "backups")
	old := filepath.Join(root, "20000101T000000.000000000Z__opencode__demo__aa")
	keep := filepath.Join(root, "20000102T000000.000000000Z__opencode__demo__bb")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "keep.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	huge, err := os.Create(filepath.Join(keep, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := huge.Truncate(maxBackupBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := huge.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.pruneBackups(filepath.Base(keep)); err == nil {
		t.Fatal("prune removed backups to make room for one that cannot be kept")
	}
	if _, err := os.ReadFile(filepath.Join(old, "keep.txt")); err != nil {
		t.Fatalf("older backup was removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(keep, "big.bin")); err != nil {
		t.Fatalf("new backup was removed: %v", err)
	}
}

func TestTargetPathsMustBeAbsoluteAndDisjoint(t *testing.T) {
	mgr, _, _ := setupTestEnv(t)
	base := t.TempDir()
	shared := filepath.Join(base, "shared")
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: shared, Mode: ModeCopy, Format: FormatSkill},
		{ID: "x", Label: "X2", Path: filepath.Join(base, "other"), Mode: ModeCopy, Format: FormatSkill},
	}); err == nil {
		t.Fatal("duplicate target id")
	}
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: shared, Mode: ModeCopy, Format: FormatSkill},
		{ID: "y", Label: "Y", Path: shared, Mode: ModeSymlink, Format: FormatSkill},
	}); err == nil {
		t.Fatal("two targets on one directory")
	}
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: "relative/skills", Mode: ModeCopy, Format: FormatSkill},
	}); err == nil {
		t.Fatal("relative target path")
	}
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: mgr.CanonicalDir, Mode: ModeCopy, Format: FormatSkill},
	}); err == nil {
		t.Fatal("target is the canonical store")
	}
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: mgr.ConfigDir, Mode: ModeCopy, Format: FormatSkill},
	}); err == nil {
		t.Fatal("target is the config directory")
	}
	nested := filepath.Join(shared, "nested")
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: shared, Mode: ModeCopy, Format: FormatSkill},
		{ID: "y", Label: "Y", Path: nested, Mode: ModeCopy, Format: FormatSkill},
	}); err == nil {
		t.Fatal("target path inside another target")
	}
	if err := mgr.UpdateTargets([]Target{
		{ID: "x", Label: "X", Path: filepath.Dir(mgr.CanonicalDir), Mode: ModeCopy, Format: FormatSkill},
	}); err == nil {
		t.Fatal("target contains the canonical store")
	}
}

func TestTomlImportKeepsTheOriginalBytes(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	dir := ExpandOK(t, mgr.GetTargets()[3].Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# hand-written\ndescription = \"Review\"\nprompt = \"Do the thing\"\nmodel = \"gemini-pro\"\n"
	src := filepath.Join(dir, "review.toml")
	if err := os.WriteFile(src, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	cands, err := mgr.ScanImportCandidates(ctx)
	if err != nil || len(cands) != 1 {
		t.Fatalf("candidates: %+v %v", cands, err)
	}
	imported, errs := mgr.ImportSelected(ctx, cands)
	if len(errs) != 0 || len(imported) != 1 {
		t.Fatalf("imported %v errs %v", imported, errs)
	}
	backups, err := mgr.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %+v %v", backups, err)
	}
	kept, err := os.ReadFile(filepath.Join(mgr.ConfigDir, "backups", backups[0].ID, "payload"))
	if err != nil || string(kept) != original {
		t.Fatalf("backed up original:\n%s\nerr %v", kept, err)
	}
	notes := mgr.Notes()
	if len(notes) != 1 || !strings.Contains(notes[0], backups[0].ID) {
		t.Fatalf("notes: %v", notes)
	}
	if _, err := os.ReadFile(filepath.Join(canonical, "review", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestDryRunCreatesNothing(t *testing.T) {
	root := t.TempDir()
	canon := filepath.Join(root, "skills")
	cfg := filepath.Join(root, "config")
	mgr, err := NewManager(WithCanonicalDir(canon), WithConfigDir(cfg), WithDryRun(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(canon); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the canonical dir: %v", err)
	}
	if _, err := os.Lstat(cfg); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the config dir: %v", err)
	}
	skills, err := ListSkills(canon)
	if err != nil || len(skills) != 0 {
		t.Fatalf("list: %v %v", skills, err)
	}
	if _, err := os.Lstat(canon); !os.IsNotExist(err) {
		t.Fatalf("list created the canonical dir: %v", err)
	}
	if _, err := mgr.Doctor(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(canon); !os.IsNotExist(err) {
		t.Fatalf("doctor created the canonical dir: %v", err)
	}
	if _, err := os.Lstat(cfg); !os.IsNotExist(err) {
		t.Fatalf("doctor created the config dir: %v", err)
	}
}
