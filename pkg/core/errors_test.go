package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectErrorPaths(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	if _, err := ExpandPath("${SKILLS_NOPE:-}"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("empty default: %v", err)
	}
	if _, err := ExpandPath("%SKILLS_NOT_A_VAR%"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("win var: %v", err)
	}
	if _, err := ExpandPath("${"); err != nil {
		t.Fatalf("bare brace: %v", err)
	}
	if _, err := ExpandPath("${NAME:-no-close"); err != nil {
		t.Fatalf("unclosed default: %v", err)
	}
	home, err := ExpandPath("~")
	if err != nil || home == "" {
		t.Fatalf("tilde: %s %v", home, err)
	}
	if _, err := CanonicalizePath("$SKILLS_NOT_A_VAR"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("canonicalize: %v", err)
	}
	if filepathIsAbs("") || !filepathIsAbs("/abs") || !filepathIsAbs(`\abs`) {
		t.Fatal("filepathIsAbs")
	}
	if IsGeminiTOMLMatch(Skill{}, "{") {
		t.Fatal("bad toml matched")
	}

	fmCases := []string{
		"---\nname: a\n",
		"---\n---\n",
		"---\n- a\n---\n",
		"---\nname:\n  - a\ndescription: d\n---\n",
		"---\nname: code-reviewer\ndescription: d\nmetadata: hi\n---\n",
		"---\nname: code-reviewer\ndescription: d\nmetadata:\n  a: b\n  a: c\n---\n",
		"---\nname: code-reviewer\ndescription:\n  - x\n---\n",
		"---\nname: code-reviewer\ndescription: d\nlicense:\n  - x\n---\n",
		"---\nname: code-reviewer\ndescription: d\ncompatibility:\n  - x\n---\n",
		"---\nname: code-reviewer\ndescription: d\nallowed-tools:\n  - x\n---\n",
	}
	for _, raw := range fmCases {
		if _, err := ParseFrontmatter(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if err := ValidateFrontmatter(Frontmatter{Name: "Bad", Description: "d"}, ""); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("validate name: %v", err)
	}

	if _, err := ListSkills("$SKILLS_NOT_A_VAR"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("list bad: %v", err)
	}
	missing := filepath.Join(dir, "created")
	if skills, err := ListSkills(missing); err != nil || len(skills) != 0 {
		t.Fatalf("list missing: %v %v", skills, err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ListSkills(filepath.Join(file, "child")); err == nil {
		t.Fatal("list under a file")
	}
	if _, err := ListSkills(file); err == nil {
		t.Fatal("list a file")
	}
	if err := os.MkdirAll(filepath.Join(missing, "Bad Name"), 0o755); err != nil {
		t.Fatal(err)
	}
	if skills, err := ListSkills(missing); err != nil || len(skills) != 0 {
		t.Fatalf("invalid folder not skipped: %v %v", skills, err)
	}
	if _, err := ParseSkillFile(filepath.Join(dir, "nope.md")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("parse missing: %v", err)
	}
	if _, err := ParseSkill("$SKILLS_NOT_A_VAR"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("parse bad: %v", err)
	}
	if _, err := CreateSkill(filepath.Join(file, "child"), "child-skill", "d", "b"); err == nil {
		t.Fatal("create under a file")
	}
	if _, err := WriteSkill(file, "---\nname: file\ndescription: d\n---\n"); err == nil {
		t.Fatal("write onto a file")
	}
	if err := RemoveSkill(dir, ".."); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("remove bad name: %v", err)
	}
	if _, err := HashFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("hash missing")
	}
	if _, err := HashDirectory(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("hash dir missing")
	}

	statePath := filepath.Join(dir, "state.json")
	if _, _, err := LoadState("$SKILLS_NOT_A_VAR"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("state path: %v", err)
	}
	if _, _, err := LoadState(dir); err == nil {
		t.Fatal("state dir")
	}
	if err := os.WriteFile(statePath, []byte("{\"version\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadState(statePath)
	if err != nil || loaded.Targets == nil {
		t.Fatalf("nil targets: %+v %v", loaded, err)
	}
	loaded.SetRecord("claude", "a", TargetRecord{Path: "p"})
	if err := loaded.SaveTo(osFS{}, filepath.Join(file, "nope")); err == nil {
		t.Fatal("save onto a file")
	}
	if !recordID(TargetRecord{}).Valid == false {
		t.Fatal("empty id")
	}

	targetsPath := filepath.Join(dir, "targets.json")
	if _, _, err := LoadTargets("$SKILLS_NOT_A_VAR"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("targets path: %v", err)
	}
	if _, _, err := LoadTargets(dir); err == nil {
		t.Fatal("targets dir")
	}
	if err := os.WriteFile(targetsPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTargets(targetsPath); err == nil {
		t.Fatal("bad targets json")
	}
	if err := ValidateTarget(Target{ID: "ok", Path: "", Mode: ModeCopy, Format: FormatSkill}); err == nil {
		t.Fatal("empty path")
	}
	if err := ValidateTarget(Target{ID: "ok", Path: dir, Mode: "nope", Format: FormatSkill}); err == nil {
		t.Fatal("bad mode")
	}
	if err := ValidateTarget(Target{ID: "ok", Path: dir, Mode: ModeCopy, Format: "nope"}); err == nil {
		t.Fatal("bad format")
	}
	bare := Target{ID: "ok", Path: dir}
	if err := ValidateTarget(bare); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if err := SaveTargets(filepath.Join(file, "nope"), []Target{bare}); err == nil {
		t.Fatal("save targets onto a file")
	}
	if _, err := GetTargetDestinationPath(Target{Path: dir, Format: FormatSkill}, ".."); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("dest name: %v", err)
	}

	canon := filepath.Join(dir, "canon")
	cfgFile := filepath.Join(dir, "cfg-file")
	if err := os.WriteFile(cfgFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(WithCanonicalDir(canon), WithConfigDir(cfgFile)); err == nil {
		t.Fatal("config file")
	}
	if _, err := NewManager(WithCanonicalDir(file), WithConfigDir(filepath.Join(dir, "cfg"))); err == nil {
		t.Fatal("canonical file")
	}

	mgr, canonical, _ := setupTestEnv(t)
	mgr.logger = nil
	mgr.log().Info("discarded")
	if _, err := mgr.enableLocked(ctx, "missing-skill", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("enable target: %v", err)
	}
	if _, err := mgr.enableLocked(ctx, "..", "claude"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("enable name: %v", err)
	}
	if _, err := mgr.enableLocked(ctx, "missing-skill", "claude"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("enable skill: %v", err)
	}
	dry := dryManager(t, mgr)
	if err := dry.UpdateTargets(mgr.GetTargets()); err != nil {
		t.Fatal(err)
	}
	release, err := mgr.acquireLock()
	if err != nil {
		t.Fatal(err)
	}
	report, err := mgr.Doctor(ctx)
	release()
	if err != nil || len(report.Checks) == 0 || report.Checks[0].Name != "lock" {
		t.Fatalf("doctor lock: %+v %v", report, err)
	}

	skill, err := CreateSkill(canonical, "err-skill", "Err", "# Err")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, skill.Name, "opencode"); err != nil {
		t.Fatal(err)
	}
	dest := GetDest(t, mgr, "opencode", skill.Name)
	if err := os.WriteFile(filepath.Join(dest, "extra.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	canHash, err := HashDirectory(skill.Path)
	if err != nil {
		t.Fatal(err)
	}
	dstHash, err := HashDirectory(dest)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := mgr.GetState().GetRecord("opencode", skill.Name)
	rec.CanonicalHash = canHash
	rec.Hash = dstHash
	mgr.GetState().SetRecord("opencode", skill.Name, rec)
	st, err := mgr.Status(ctx, skill.Name, "opencode")
	if err != nil || st.Code != StatusConflict || !stringsContains(st.Message, "baseline") {
		t.Fatalf("inconsistent baseline: %+v %v", st, err)
	}

	src := filepath.Join(GetDest(t, mgr, "claude", "parent"), "outside")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	if err := os.Symlink(skill.Path, filepath.Join(ExpandOK(t, mgr.GetTargets()[0].Path), "link-skill")); err != nil {
		t.Fatal(err)
	}
	err = mgr.ImportCandidate(ctx, ImportCandidate{
		TargetID: "claude", SkillName: "link-skill", SourcePath: filepath.Join(ExpandOK(t, mgr.GetTargets()[0].Path), "link-skill"),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("import symlink: %v", err)
	}
	if err := mgr.ImportCandidate(ctx, ImportCandidate{TargetID: "missing", SkillName: "err-skill", SourcePath: skill.Path}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("import target: %v", err)
	}
	if err := mgr.ImportCandidate(ctx, ImportCandidate{TargetID: "claude", SkillName: "..", SourcePath: skill.Path}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("import name: %v", err)
	}
	dryImp := dryManager(t, mgr)
	cands, err := mgr.ScanImportCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = cands
	if err := dryImp.ImportCandidate(ctx, ImportCandidate{
		TargetID: "opencode", SkillName: "not-there", SourcePath: dest, Description: "kept",
	}); err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrExists) {
		// dry-run still checks the source. dest is inside opencode, and the
		// canonical skill does not use this name, so a directory source is valid.
		t.Fatalf("dry import: %v", err)
	}

	var buf bytes.Buffer
	stop, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := mgr.ImportZip(stop, bytes.NewReader(nil), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("zip cancel: %v", err)
	}
	if _, err := mgr.ImportZip(ctx, bytes.NewReader([]byte("not a zip")), 9); err == nil {
		t.Fatal("bad zip")
	}
	if err := mgr.ExportZip(ctx, []string{".."}, &buf); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("export name: %v", err)
	}
	alias := filepath.Join(canonical, "alias-skill")
	if err := os.Symlink(skill.Path, alias); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ExportZip(ctx, []string{"alias-skill"}, &buf); err == nil {
		t.Fatal("export symlink dir")
	}

	if _, _, err := IsSymlink(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("issymlink missing")
	}
	ok, target, err := IsSymlink(file)
	if err != nil || ok || target != "" {
		t.Fatalf("issymlink file: %v %s %v", ok, target, err)
	}
	if err := CopyFile("$SKILLS_NOT_A_VAR", filepath.Join(dir, "out")); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("copyfile: %v", err)
	}
	if err := CopyDirectory("$SKILLS_NOT_A_VAR", filepath.Join(dir, "out")); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("copydir: %v", err)
	}
	if _, err := CreateSymlinkOrFallback("$SKILLS_NOT_A_VAR", filepath.Join(dir, "link")); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("link bad: %v", err)
	}
	copied := filepath.Join(dir, "copied.txt")
	if err := CopyFile(file, copied); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileWatcher(nil, 0); err == nil {
		t.Fatal("nil watcher")
	}
	w, err := NewFileWatcher(mgr, 0)
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(canonical, "watched-sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	w.RefreshPaths()
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	w.RefreshPaths()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.backupLocked("opencode", skill.Name, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("backup missing")
	}
	if _, err := mgr.ListBackups(); err != nil {
		t.Fatal(err)
	}
	bogus := filepath.Join(mgr.ConfigDir, "backups", "not-a-real-backup")
	if err := os.MkdirAll(bogus, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bogus, "meta.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, err := mgr.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range listed {
		if b.ID == "not-a-real-backup" {
			t.Fatal("listed a corrupt backup")
		}
	}
	if err := mgr.Restore(ctx, "not-a-real-backup"); err == nil {
		t.Fatal("restored corrupt backup")
	}
}

func ExpandOK(t *testing.T, path string) string {
	t.Helper()
	got, err := ExpandPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func stringsContains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || (func() bool {
		for i := 0; i+len(part) <= len(s); i++ {
			if s[i:i+len(part)] == part {
				return true
			}
		}
		return false
	})())
}
