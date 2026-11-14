package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SKILLS_LOCK_CHILD") == "1" {
		os.Exit(lockChild())
	}
	orig, err := os.UserHomeDir()
	if err != nil || orig == "" {
		fmt.Fprintln(os.Stderr, "tests require a home directory to prove they do not touch it")
		os.Exit(1)
	}
	fake, err := os.MkdirTemp("", "skills-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Setenv("SKILLS_ORIGINAL_HOME", orig)
	_ = os.Setenv("HOME", fake)
	_ = os.Setenv("USERPROFILE", fake)
	_ = os.Setenv("SKILLS_TEST_MODE", "1")
	_ = os.Unsetenv("XDG_CONFIG_HOME")
	_ = os.Unsetenv("XDG_DATA_HOME")
	_ = os.Unsetenv("XDG_STATE_HOME")
	_ = os.Unsetenv("CODEX_HOME")
	code := m.Run()
	_ = os.RemoveAll(fake)
	os.Exit(code)
}

func lockChild() int {
	dir := os.Getenv("SKILLS_LOCK_DIR")
	mgr, err := NewManager(
		WithCanonicalDir(filepath.Join(dir, "canon")),
		WithConfigDir(filepath.Join(dir, "cfg")),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	release, err := mgr.acquireLock()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer release()
	fmt.Println("held")
	time.Sleep(20 * time.Second)
	return 0
}

func TestCrossProcessLock(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(
		WithCanonicalDir(filepath.Join(dir, "canon")),
		WithConfigDir(filepath.Join(dir, "cfg")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSkill(mgr.CanonicalDir, "locked-skill", "Locked", "# Body"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "SKILLS_LOCK_CHILD=1", "SKILLS_LOCK_DIR="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	buf := make([]byte, 16)
	if _, err := stdout.Read(buf); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.Enable(context.Background(), "locked-skill", "claude")
	if err == nil {
		t.Fatal("expected the second process to be locked out")
	}
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestHermeticGuardRefusesRealHome(t *testing.T) {
	orig := os.Getenv("SKILLS_ORIGINAL_HOME")
	if orig == "" {
		t.Fatal("SKILLS_ORIGINAL_HOME is not set")
	}
	_, err := NewManager(WithCanonicalDir(filepath.Join(orig, "skills-manager-should-not-create")))
	if err == nil || !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected refusal of the real home, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(orig, "skills-manager-should-not-create")); !os.IsNotExist(statErr) {
		t.Fatalf("real home was touched: %v", statErr)
	}
}
