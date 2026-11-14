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

func TestBrokenSymlinkIsRepaired(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "repair-skill", "Repair", "# Repair")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "claude", skill.Name)
	fsys := &onceBadReadlink{dest: dest}
	mgr.fs = fsys
	st, err := mgr.Status(ctx, skill.Name, "claude")
	if err != nil || st.Code != StatusBroken {
		t.Fatalf("status: %+v %v", st, err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(dest)
	if err != nil || !linksTo(dest, got, skill.Path) {
		t.Fatalf("link after repair: %s %v", got, err)
	}
}

func TestSymlinkTargetReplacesDriftedCopy(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "mode-switch", "Mode", "# Mode")
	if err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "claude", skill.Name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(osFS{}, skill.Path, skill.Path, dest); err != nil {
		t.Fatal(err)
	}
	id, err := fileIdentity(dest)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashDirectory(dest)
	if err != nil {
		t.Fatal(err)
	}
	canHash, err := HashDirectory(skill.Path)
	if err != nil {
		t.Fatal(err)
	}
	mgr.GetState().SetRecord("claude", skill.Name, TargetRecord{
		Mode: ModeCopy, Format: FormatSkill, Hash: hash, CanonicalHash: canHash,
		Path: dest, Device: id.Dev, Inode: id.Ino, IdentityValid: id.Valid,
	})
	if err := mgr.SaveState(); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSkill(skill.Path, skillDoc("mode-switch", "Mode changed", "# Changed")); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Status(ctx, skill.Name, "claude")
	if err != nil || st.Code != StatusDrifted {
		t.Fatalf("status: %+v %v", st, err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(dest)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected a symlink, got %v %v", info, err)
	}
	backups, err := mgr.ListBackups()
	if err != nil || len(backups) == 0 {
		t.Fatalf("expected a backup of the copy: %v %v", backups, err)
	}
}

func TestInjectedFaultsLeaveOriginals(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "fault-skill", "Fault", "# Fault")
	if err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", skill.Name)
	mgr.fs = failOpFS{op: "MkdirAll", suffix: "staging", err: syscall.EACCES}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err == nil {
		t.Fatal("expected staging failure")
	}

	mgr.fs = osFS{}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSkill(skill.Path, skillDoc("fault-skill", "Fault changed", "# Changed")); err != nil {
		t.Fatal(err)
	}
	mgr.fs = failOpFS{op: "Rename", suffix: filepath.Base(dest), err: syscall.EPERM}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err == nil {
		t.Fatal("expected rename failure")
	}
	got, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || string(got) != string(before) {
		t.Fatalf("dest changed after failed replace: %s", got)
	}

	mgr.fs = failOpFS{op: "RenameExclusive", err: syscall.EEXIST}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err == nil {
		t.Fatal("expected publish failure")
	}
	got, err = os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || string(got) != string(before) {
		t.Fatalf("dest changed after failed publish: %s", got)
	}

	mgr.fs = failOpFS{op: "RemoveTree", suffix: ".hold-", err: syscall.EIO}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); !errors.Is(err, ErrConflict) {
		t.Fatalf("hold delete: %v", err)
	}
	got, err = os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || string(got) != string(before) {
		t.Fatalf("dest changed after hold failure: %s", got)
	}

	mgr.fs = osFS{}
	link := GetDest(t, mgr, "claude", skill.Name)
	mgr.fs = failOpFS{op: "Symlink", err: syscall.EIO}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err == nil {
		t.Fatal("expected symlink failure")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("link left behind: %v", err)
	}
	_ = dest
}

func TestDisableClearsMissingRecordAndDryRunDoesNot(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "gone-skill", "Gone", "# Gone")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "claude", skill.Name)
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	dry := dryManager(t, mgr)
	if err := dry.Disable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	if !mgr.GetState().IsTracked("claude", skill.Name) {
		t.Fatal("dry-run cleared state")
	}
	if err := mgr.Disable(ctx, skill.Name, "claude"); err != nil {
		t.Fatal(err)
	}
	if mgr.GetState().IsTracked("claude", skill.Name) {
		t.Fatal("state still tracks a missing link")
	}
}

func TestImportFailuresRestoreSource(t *testing.T) {
	mgr, _, _ := setupTestEnv(t)
	ctx := context.Background()
	cancel, stop := context.WithCancel(ctx)
	stop()
	_, errs := mgr.ImportSelected(cancel, []ImportCandidate{{SkillName: "x", TargetID: "claude", SourcePath: "x"}})
	if len(errs) != 1 || !errors.Is(errs[0], context.Canceled) {
		t.Fatalf("canceled import: %v", errs)
	}

	claude, err := ExpandPath(mgr.GetTargets()[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(claude, "locked-import")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(skillDoc("locked-import", "Locked", "# Locked"))
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(claude, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(claude, 0o755) })
	err = mgr.ImportCandidate(ctx, ImportCandidate{TargetID: "claude", SkillName: "locked-import", SourcePath: src})
	if err == nil {
		t.Fatal("import succeeded without write permission")
	}
	if err := os.Chmod(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(src, "SKILL.md")); err != nil {
		t.Fatalf("source missing after failed import: %v", err)
	}

	gemini, err := ExpandPath(mgr.GetTargets()[3].Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gemini, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlPath := filepath.Join(gemini, "toml-fail.toml")
	if err := os.WriteFile(tomlPath, []byte("description = \"Toml\"\nprompt = \"Hi\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr.fs = failOpFS{op: "WriteFileExclusive", err: syscall.ENOSPC}
	err = mgr.ImportCandidate(ctx, ImportCandidate{
		TargetID: "gemini", SkillName: "toml-fail", SourcePath: tomlPath, IsToml: true,
	})
	if err == nil {
		t.Fatal("toml import survived a failed write")
	}
	if _, err := os.ReadFile(tomlPath); err != nil {
		t.Fatalf("toml source missing: %v", err)
	}
}

func TestSyncReportsTargetErrors(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	if _, err := CreateSkill(canonical, "sync-skill", "Sync", "# Sync"); err != nil {
		t.Fatal(err)
	}
	targets := append(mgr.GetTargets(), Target{
		ID: "bad", Label: "Bad", Path: "$SKILLS_MISSING", Mode: ModeSymlink, Format: FormatSkill,
	})
	if err := SaveTargets(mgr.TargetsPath, targets); err != nil {
		t.Fatal(err)
	}
	report, err := mgr.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) == 0 {
		t.Fatalf("expected a target error: %+v", report)
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(canonical, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(canonical, 0o755) })
		if _, err := mgr.Sync(ctx); err == nil {
			t.Fatal("sync read an unreadable canonical dir")
		}
	}
}

func TestBackupAndExportEdges(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "edge-export", "Edge", "# Edge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.backupLocked("opencode", "NOT-VALID", skill.Path); err == nil {
		t.Fatal("backup accepted a bad name")
	}
	dry := dryManager(t, mgr)
	if id, err := dry.backupLocked("opencode", skill.Name, skill.Path); err != nil || id != "" {
		t.Fatalf("dry backup: %s %v", id, err)
	}
	mgr.fs = failOpFS{op: "CopyFile", err: syscall.EIO}
	if _, err := mgr.backupLocked("opencode", skill.Name, skill.Path); err == nil {
		t.Fatal("backup ignored a copy failure")
	}
	mgr.fs = osFS{}
	link := filepath.Join(canonical, "linked-skill")
	if err := os.Symlink(skill.Path, link); err != nil {
		t.Fatal(err)
	}
	var buf bytesBuffer
	if err := mgr.ExportZip(ctx, []string{"linked-skill"}, &buf); err == nil {
		t.Fatal("exported a symlinked skill directory")
	}
	stop, cancel := context.WithCancel(ctx)
	cancel()
	if err := mgr.ExportZip(stop, nil, &buf); !errors.Is(err, context.Canceled) {
		t.Fatalf("export cancel: %v", err)
	}
}

type bytesBuffer struct{ b []byte }

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.b = append(b.b, p...)
	return len(p), nil
}

type onceBadReadlink struct {
	osFS
	dest string
	seen int
}

func (o *onceBadReadlink) Readlink(name string) (string, error) {
	if name == o.dest {
		o.seen++
		if o.seen == 1 {
			return "", syscall.EIO
		}
	}
	return os.Readlink(name)
}

type failOpFS struct {
	osFS
	op     string
	suffix string
	err    error
}

func (f failOpFS) fail(op, path string) error {
	if f.op != op {
		return nil
	}
	if f.suffix != "" && !stringsHas(path, f.suffix) {
		return nil
	}
	return f.err
}

func stringsHas(path, part string) bool {
	return strings.Contains(path, part)
}

func (f failOpFS) MkdirAll(path string, perm os.FileMode) error {
	if err := f.fail("MkdirAll", path); err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

func (f failOpFS) Rename(oldpath, newpath string) error {
	if err := f.fail("Rename", oldpath); err != nil {
		return err
	}
	if err := f.fail("Rename", newpath); err != nil {
		return err
	}
	return os.Rename(oldpath, newpath)
}

func (f failOpFS) RenameExclusive(oldpath, newpath string) error {
	if err := f.fail("RenameExclusive", newpath); err != nil {
		return err
	}
	return renameNoReplace(oldpath, newpath)
}

func (f failOpFS) RemoveTree(path string, id FileID) error {
	if err := f.fail("RemoveTree", path); err != nil {
		return err
	}
	return removeTreeNoFollow(path, id)
}

func (f failOpFS) Symlink(oldname, newname string) error {
	if err := f.fail("Symlink", newname); err != nil {
		return err
	}
	return os.Symlink(oldname, newname)
}

func (f failOpFS) CopyFile(src, dst string) error {
	if err := f.fail("CopyFile", src); err != nil {
		return err
	}
	return copyFileStream(src, dst)
}

func (f failOpFS) WriteFileExclusive(name string, data []byte, perm os.FileMode) error {
	if err := f.fail("WriteFileExclusive", name); err != nil {
		return err
	}
	return f.osFS.WriteFileExclusive(name, data, perm)
}
