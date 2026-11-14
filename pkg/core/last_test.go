package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLimitHelpers(t *testing.T) {
	if err := zipTooLarge(maxZipEntries+1, 0); err == nil {
		t.Fatal("entry cap")
	}
	if err := zipTooLarge(1, maxZipUncompressed+1); err == nil {
		t.Fatal("byte cap")
	}
	if err := zipRatio(0, zipRatioMinUnpacked*2); err != nil {
		t.Fatal(err)
	}
	mgr, canonical, _ := setupTestEnv(t)
	skill, err := CreateSkill(canonical, "edge-call", "Edge", "# Edge")
	if err != nil {
		t.Fatal(err)
	}
	target, ok := mgr.GetTargetByID("opencode")
	if !ok {
		t.Fatal("target")
	}
	dest, err := GetTargetDestinationPath(target, skill.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.enableCopy(target, skill.Name, skill.Path, dest, CellStatus{Code: StatusEnabled, Message: "no"}); err == nil {
		t.Fatal("enableCopy accepted enabled")
	}
	gemini, _ := mgr.GetTargetByID("gemini")
	if _, err := mgr.enableTOML(gemini, skill.Name, skill.Path, dest+".toml", CellStatus{Code: StatusEnabled, Message: "no"}); err == nil {
		t.Fatal("enableTOML accepted enabled")
	}
	if err := mgr.disableLocked("..", "claude"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("disable: %v", err)
	}
	for _, name := range []string{"", "..", "a/../../b"} {
		if _, err := safeZipName(name); err == nil {
			t.Fatalf("zip name %q", name)
		}
	}
	if _, err := mgr.enableSymlink(target, skill.Name, skill.Path, dest, CellStatus{Code: StatusConflict, Message: "no"}); err == nil {
		t.Fatal("enableSymlink accepted a conflict")
	}
	if _, err := mgr.enableSymlink(target, skill.Name, skill.Path, dest, CellStatus{Code: StatusBroken, Message: "broken"}); err == nil {
		t.Fatal("enableSymlink replaced a missing path")
	}
	if os.Geteuid() != 0 {
		hidden := filepath.Join(t.TempDir(), "hidden")
		if err := os.WriteFile(hidden, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(hidden, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := HashFile(hidden); err == nil {
			t.Fatal("hashed an unreadable file")
		}
	}
}

func TestRemainingStatusAndDoctorBranches(t *testing.T) {
	if fallbackDesc(ImportCandidate{SkillName: "named", Description: "kept"}) != "kept" {
		t.Fatal("description fallback")
	}
	if err := removeImported(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("remove missing")
	}
	over := filepath.Join(t.TempDir(), "over.txt")
	if _, err := writeLimited(over, strings.NewReader(strings.Repeat("a", 8)), 3); err == nil {
		t.Fatal("writeLimited accepted an oversized reader")
	}

	mgr, canonical, _ := setupTestEnv(t)
	ctx := context.Background()
	stop, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := mgr.Status(stop, "whatever", "claude"); !errors.Is(err, context.Canceled) {
		t.Fatalf("status cancel: %v", err)
	}
	if _, err := mgr.Status(ctx, "..", "claude"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("status name: %v", err)
	}
	skill, err := CreateSkill(canonical, "branch-skill", "Branch", "# Branch")
	if err != nil {
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
		t.Fatal("record")
	}
	rec.Fallback = true
	mgr.GetState().SetRecord("opencode", skill.Name, rec)
	st, err := mgr.Status(ctx, skill.Name, "opencode")
	if err != nil || !strings.Contains(st.Message, "fallback") {
		t.Fatalf("fallback message: %+v %v", st, err)
	}

	if err := os.Remove(filepath.Join(skill.Path, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	st, err = mgr.Status(ctx, skill.Name, "gemini")
	if err != nil || st.Code != StatusBroken {
		t.Fatalf("toml parse: %+v %v", st, err)
	}
	if _, err := WriteSkill(skill.Path, skillDoc("branch-skill", "Branch", "# Branch")); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		toml := GetDest(t, mgr, "gemini", skill.Name)
		if err := os.Chmod(toml, 0); err != nil {
			t.Fatal(err)
		}
		st, err = mgr.Status(ctx, skill.Name, "gemini")
		_ = os.Chmod(toml, 0o644)
		if err != nil || st.Code != StatusBroken {
			t.Fatalf("toml read: %+v %v", st, err)
		}
		if err := os.Chmod(skill.Path, 0); err != nil {
			t.Fatal(err)
		}
		st, err = mgr.Status(ctx, skill.Name, "opencode")
		_ = os.Chmod(skill.Path, 0o755)
		if err != nil || st.Code != StatusBroken {
			t.Fatalf("copy hash: %+v %v", st, err)
		}
	}

	outside := t.TempDir()
	if err := mgr.ImportCandidate(ctx, ImportCandidate{
		TargetID: "claude", SkillName: "branch-skill", SourcePath: outside,
	}); !errors.Is(err, ErrInvalidPath) && !errors.Is(err, ErrExists) {
		t.Fatalf("outside import: %v", err)
	}
	claude := ExpandOK(t, mgr.GetTargets()[0].Path)
	dup := filepath.Join(claude, "branch-skill")
	if err := os.MkdirAll(dup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ImportCandidate(ctx, ImportCandidate{
		TargetID: "claude", SkillName: "branch-skill", SourcePath: dup,
	}); !errors.Is(err, ErrExists) {
		t.Fatalf("exists import: %v", err)
	}

	file := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets := mgr.GetTargets()
	targets = append(targets, Target{ID: "filetarget", Label: "File", Path: file, Mode: ModeSymlink, Format: FormatSkill})
	if err := mgr.UpdateTargets(targets); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSkill(skill.Path, skillDoc("branch-skill", "Branch drifted", "# Drifted")); err != nil {
		t.Fatal(err)
	}
	drifted, err := mgr.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sawDrift bool
	for _, check := range drifted.Checks {
		if strings.Contains(check.Detail, "behind") {
			sawDrift = true
		}
	}
	if !sawDrift {
		t.Fatalf("doctor missed drift: %+v", drifted.Checks)
	}
	if err := RemoveSkill(canonical, skill.Name); err != nil {
		t.Fatal(err)
	}
	report, err := mgr.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sawOrphan, sawFile bool
	for _, check := range report.Checks {
		if strings.HasPrefix(check.Name, "orphan:") {
			sawOrphan = true
		}
		if check.Name == "target:filetarget" && check.Status == "fail" {
			sawFile = true
		}
	}
	if !sawOrphan || !sawFile {
		t.Fatalf("doctor checks: %+v", report.Checks)
	}
	badZip := zipBytes(t, []zipFile{{
		name: "bad-skill/SKILL.md",
		body: []byte("not frontmatter"),
	}})
	if _, err := mgr.ImportZip(ctx, bytes.NewReader(badZip), int64(len(badZip))); err == nil {
		t.Fatal("imported a skill without frontmatter")
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(mgr.ConfigDir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(mgr.ConfigDir, 0o755) })
		locked, err := mgr.Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var sawConfig bool
		for _, check := range locked.Checks {
			if check.Name == "config" && check.Status == "fail" {
				sawConfig = true
			}
		}
		if !sawConfig {
			t.Fatalf("writable check: %+v", locked.Checks)
		}
	}
}
