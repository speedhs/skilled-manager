package core_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/speedhs/skilled-manager/pkg/core"
)

// TestReadmeSnippet compiles the calls shown in the README.
func TestReadmeSnippet(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mgr, err := core.NewManager(
		core.WithCanonicalDir(filepath.Join(dir, "skills")),
		core.WithConfigDir(filepath.Join(dir, "config")),
	)
	if err != nil {
		t.Fatal(err)
	}
	targets := mgr.GetTargets()
	for i := range targets {
		targets[i].Path = filepath.Join(dir, targets[i].ID)
	}
	if err := mgr.UpdateTargets(targets); err != nil {
		t.Fatal(err)
	}
	skill, err := core.CreateSkill(mgr.CanonicalDir, "code-reviewer", "Reviews pull requests", "# Review\n")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "code-reviewer" {
		t.Fatalf("name %s", skill.Name)
	}
	if _, err := core.ListSkills(mgr.CanonicalDir); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Enable(ctx, "code-reviewer", "claude"); err != nil {
		t.Fatal(err)
	}
	status, err := mgr.Status(ctx, "code-reviewer", "claude")
	if err != nil || status.Code != core.StatusEnabled {
		t.Fatalf("status %+v %v", status, err)
	}
	if _, err := mgr.Sync(ctx); err != nil {
		t.Fatal(err)
	}
}
