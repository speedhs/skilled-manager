package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCrossDevicePublishAndCollisions(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "cross-device", "Cross", "# Cross")
	if err != nil {
		t.Fatal(err)
	}
	dest, err := GetTargetDestinationPath(mgr.GetTargets()[2], skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	mgr.fs = exdevFS{dest: dest}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	mgr.fs = osFS{}
	if err := mgr.Disable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}

	skill2, err := CreateSkill(canonical, "cross-fail", "Cross", "# Cross")
	if err != nil {
		t.Fatal(err)
	}
	dest2, err := GetTargetDestinationPath(mgr.GetTargets()[2], skill2.Name)
	if err != nil {
		t.Fatal(err)
	}
	mgr.fs = exdevFS{dest: dest2, copyFail: true}
	if _, err := mgr.Enable(ctx, skill2.Name, "opencode"); err == nil {
		t.Fatal("expected copy fallback to fail")
	}
	if _, err := os.Lstat(dest2); !os.IsNotExist(err) {
		t.Fatalf("partial copy left behind: %v", err)
	}

	tomlSkill, err := CreateSkill(canonical, "appear-toml", "Appear", "Hello")
	if err != nil {
		t.Fatal(err)
	}
	tomlDest, err := GetTargetDestinationPath(mgr.GetTargets()[3], tomlSkill.Name)
	if err != nil {
		t.Fatal(err)
	}
	mgr.fs = appearFS{dest: tomlDest}
	if _, err := mgr.Enable(ctx, tomlSkill.Name, "gemini"); !errors.Is(err, ErrConflict) {
		t.Fatalf("toml collision: %v", err)
	}

	linkSkill, err := CreateSkill(canonical, "appear-link", "Appear", "# Link")
	if err != nil {
		t.Fatal(err)
	}
	linkDest, err := GetTargetDestinationPath(mgr.GetTargets()[0], linkSkill.Name)
	if err != nil {
		t.Fatal(err)
	}
	mgr.fs = appearFS{dest: linkDest}
	if _, err := mgr.Enable(ctx, linkSkill.Name, "claude"); !errors.Is(err, ErrConflict) {
		t.Fatalf("symlink collision: %v", err)
	}
}

func TestImportWithoutSkillFileAndBadTOML(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	claude, err := ExpandPath(mgr.GetTargets()[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(claude, "bare-skill")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	gemini, err := ExpandPath(mgr.GetTargets()[3].Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gemini, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(gemini, "bad-toml.toml")
	if err := os.WriteFile(bad, []byte("description = \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cands, err := mgr.ScanImportCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bareCand []ImportCandidate
	for _, c := range cands {
		if c.SkillName == "bare-skill" {
			bareCand = append(bareCand, c)
		}
	}
	if len(bareCand) != 1 {
		t.Fatalf("bare candidate: %+v", cands)
	}
	imported, errs := mgr.ImportSelected(ctx, append(bareCand, ImportCandidate{
		TargetID: "gemini", SkillName: "bad-toml", SourcePath: bad, IsToml: true,
	}))
	if len(imported) != 1 || len(errs) != 1 {
		t.Fatalf("imported %v errs %v", imported, errs)
	}
	body, err := os.ReadFile(filepath.Join(canonical, "bare-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "bare-skill") {
		t.Fatalf("synthesized skill: %s %v", body, err)
	}
	if _, err := os.Lstat(bad); err != nil {
		t.Fatal("bad toml was removed")
	}
	info, err := os.Lstat(bare)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("import did not link the original path back: %v", err)
	}
	matches, err := filepath.Glob(bare + ".import-bak-*")
	if err != nil || len(matches) != 0 {
		t.Fatalf("import backup left behind: %v", matches)
	}
}

func TestRestoreConflictsAndDryRun(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "restore-skill", "Restore", "# Restore")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", skill.Name)
	id, err := mgr.backupLocked("opencode", skill.Name, dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Restore(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	dry := dryManager(t, mgr)
	if err := dry.Restore(ctx, id); err != nil {
		t.Fatalf("dry-run restore: %v", err)
	}
	foreign := t.TempDir()
	if err := os.RemoveAll(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, dest); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Restore(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore over symlink: %v", err)
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Restore(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore over foreign dir: %v", err)
	}
	if err := mgr.Restore(ctx, "missing-backup"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing backup: %v", err)
	}
	if err := mgr.Disable(ctx, skill.Name, "opencode"); err == nil {
		t.Fatal("disable deleted a foreign directory")
	}
}

func TestSkillEdgeCases(t *testing.T) {
	dir := t.TempDir()
	if _, err := CreateSkill(dir, "edge-skill", "   ", "body"); !errors.Is(err, ErrEmptyDescription) {
		t.Fatalf("empty description: %v", err)
	}
	skill, err := CreateSkill(dir, "edge-skill", "Edge", "# Edge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSkill(skill.Path, "---\nname: other\ndescription: d\n---\n"); err == nil {
		t.Fatal("name mismatch accepted")
	}
	if err := os.Symlink(skill.Path, filepath.Join(dir, "link-skill")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSkill(dir, "link-skill"); !errors.Is(err, ErrConflict) {
		t.Fatalf("remove symlink: %v", err)
	}
	if err := RemoveSkill(dir, "missing-skill"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove missing: %v", err)
	}
	if err := RemoveSkill(dir, "edge-skill"); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(nested, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ListSkills(nested); err == nil {
		t.Fatal("listed a file")
	}
	if _, err := ParseSkillFile(nested); err == nil {
		t.Fatal("parsed a non-markdown file as skill? frontmatter should fail")
	}
	link := filepath.Join(dir, "skill-link")
	if err := os.Symlink(nested, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSkillFile(link); err == nil {
		t.Fatal("followed a symlink")
	}
	if _, err := NewManager(WithCanonicalDir("$SKILLS_DOES_NOT_EXIST")); err == nil {
		t.Fatal("bad canonical path")
	}
	mgr, _, _ := setupTestEnv(t)
	if err := mgr.UpdateTargets([]Target{{ID: "bad id", Path: dir}}); err == nil {
		t.Fatal("bad target")
	}
	cancel, stop := context.WithCancel(context.Background())
	stop()
	if _, err := mgr.Sync(cancel); !errors.Is(err, context.Canceled) {
		t.Fatalf("sync cancel: %v", err)
	}
	var buf bytes.Buffer
	if err := mgr.ExportZip(context.Background(), []string{"missing"}, &buf); !errors.Is(err, ErrNotFound) {
		t.Fatalf("export missing: %v", err)
	}
}

func TestUnreadableDeploymentIsBroken(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read mode 000 directories")
	}
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "unreadable", "Hidden", "# Hidden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", skill.Name)
	if err := os.Chmod(dest, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dest, 0o755) })
	st, err := mgr.Status(ctx, skill.Name, "opencode")
	if err != nil || st.Code != StatusBroken {
		t.Fatalf("status: %+v %v", st, err)
	}
	report, err := mgr.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != JSONSchemaVersion {
		t.Fatal(report.SchemaVersion)
	}
}

func TestDirectFilesystemHelpers(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "a.txt")
	var f faultFS
	if err := f.WriteFileExclusive(name, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.WriteFileExclusive(name, []byte("b"), 0o644); err == nil {
		t.Fatal("exclusive overwrite")
	}
	rc, err := f.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil || string(body) != "a" {
		t.Fatalf("open: %q %v", body, err)
	}
	other := filepath.Join(dir, "b.txt")
	if err := f.Rename(name, other); err != nil {
		t.Fatal(err)
	}
	id, err := f.Identity(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.RemoveTree(other, id); err != nil {
		t.Fatal(err)
	}
	blocked := faultFS{fail: map[string]error{
		"WriteFileAtomic:" + name:    syscall.EIO,
		"WriteFileExclusive:" + name: syscall.EIO,
		"CopyFile:" + name:           syscall.EIO,
		"Symlink:" + other:           syscall.EIO,
		"Rename:" + other:            syscall.EIO,
		"RenameExclusive:" + other:   syscall.EXDEV,
		"RemoveTree:" + other:        syscall.EIO,
	}}
	if err := blocked.WriteFileAtomic(name, []byte("z"), 0o644); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	if err := blocked.WriteFileExclusive(name, []byte("z"), 0o644); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	if err := blocked.CopyFile(name, other); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	if err := blocked.Symlink(name, other); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	if err := blocked.Rename(name, other); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	if err := blocked.RenameExclusive(name, other); !errors.Is(err, syscall.EXDEV) {
		t.Fatal(err)
	}
	if err := blocked.RemoveTree(other, FileID{}); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	parentFile := filepath.Join(dir, "parent-file")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(parentFile, "child"), []byte("nope"), 0o644); err == nil {
		t.Fatal("writeAtomic into a file parent")
	}
	if err := CopyFile(filepath.Join(dir, "missing"), other); err == nil {
		t.Fatal("copy missing")
	}
	t.Setenv("SKILLS_BACKSLASH", `\`)
	if _, err := ExpandPath("$SKILLS_BACKSLASH"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("backslash root: %v", err)
	}
	nested := filepath.Join(dir, "tree")
	if err := os.MkdirAll(filepath.Join(nested, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "sub", "f.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f.txt", filepath.Join(nested, "sub", "rel")); err != nil {
		t.Fatal(err)
	}
	id, err = fileIdentity(nested)
	if err != nil {
		t.Fatal(err)
	}
	if err := removeTreeNoFollow(nested, id); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	mgr, canonical, _ := setupTestEnv(t)
	if _, err := CreateSkill(canonical, "zip-bad", "Bad", "# Bad"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(canonical, "zip-bad", "leak")); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ExportZip(context.Background(), nil, &out); err == nil {
		t.Fatal("exported a symlink")
	}
	payload := zipBytes(t, []zipFile{{name: "not a skill/SKILL.md", body: []byte("x")}})
	if _, err := mgr.ImportZip(context.Background(), bytes.NewReader(payload), int64(len(payload))); err == nil {
		t.Fatal("imported an invalid skill name")
	}
}

func TestDriftedReplaceRefusesIdentityChange(t *testing.T) {
	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	skill, err := CreateSkill(canonical, "swap-skill", "Swap", "# Swap")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", skill.Name)
	if _, err := WriteSkill(skill.Path, skillDoc("swap-skill", "Changed", "# Changed")); err != nil {
		t.Fatal(err)
	}
	hold := dest + "-hold"
	if err := os.Rename(dest, hold); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := mgr.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) == 0 && len(report.Conflicts) == 0 {
		t.Fatalf("sync accepted a swapped directory: %+v", report)
	}
	body, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || string(body) != "foreign" {
		t.Fatalf("foreign file changed: %q %v", body, err)
	}
	if _, err := os.ReadFile(filepath.Join(hold, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

type exdevFS struct {
	osFS
	dest     string
	copyFail bool
}

func (e exdevFS) RenameExclusive(oldpath, newpath string) error {
	if newpath == e.dest {
		return syscall.EXDEV
	}
	return e.osFS.RenameExclusive(oldpath, newpath)
}

func (e exdevFS) CopyFile(src, dst string) error {
	if e.copyFail && strings.HasPrefix(dst, e.dest) {
		return syscall.EIO
	}
	return copyFileStream(src, dst)
}

type appearFS struct {
	osFS
	dest string
}

func (a appearFS) Lstat(name string) (fs.FileInfo, error) {
	if name == a.dest {
		return nil, os.ErrNotExist
	}
	return a.osFS.Lstat(name)
}

func (a appearFS) WriteFileExclusive(name string, data []byte, perm fs.FileMode) error {
	if name == a.dest {
		if err := os.WriteFile(name, []byte("occupied"), 0o644); err != nil {
			return err
		}
	}
	return a.osFS.WriteFileExclusive(name, data, perm)
}

func TestSizeAndRegularFileGuards(t *testing.T) {
	if _, err := nonNegative(-1); err == nil {
		t.Fatal("negative size accepted")
	}
	n, err := nonNegative(4)
	if err != nil || n != 4 {
		t.Fatalf("nonNegative(4) = %d %v", n, err)
	}
	dir := t.TempDir()
	if _, err := HashFile(dir); err == nil {
		t.Fatal("directory was hashed as a regular file")
	}
	if _, err := openRegular(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing file opened")
	}
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openRegular(link); err == nil {
		t.Fatal("symlink opened")
	}
}

func (a appearFS) Symlink(oldname, newname string) error {
	if newname == a.dest {
		if err := os.WriteFile(newname, []byte("occupied"), 0o644); err != nil {
			return err
		}
	}
	return a.osFS.Symlink(oldname, newname)
}
