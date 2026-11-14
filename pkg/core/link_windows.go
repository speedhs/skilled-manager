//go:build windows

package core

import (
	"fmt"
	"os/exec"
	"strings"
)

func createWindowsJunction(canonicalPath, targetPath string) error {
	if err := safeWindowsArg(canonicalPath); err != nil {
		return err
	}
	if err := safeWindowsArg(targetPath); err != nil {
		return err
	}
	// cmd.exe parses the tail as a command line. Quotes keep spaces intact.
	// safeWindowsArg already rejected quotes and shell metacharacters.
	cmd := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", quoteCmd(targetPath), quoteCmd(canonicalPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func quoteCmd(s string) string {
	return `"` + s + `"`
}

func linkFallback(fsys fileSystem, canonical, dest string, _ error) (LinkResult, error) {
	if err := createWindowsJunction(canonical, dest); err == nil {
		return LinkResult{Type: LinkTypeJunction, Fallback: true, Message: "created a directory junction because symlink was denied"}, nil
	}
	if err := copyTree(fsys, canonical, canonical, dest); err == nil {
		return LinkResult{Type: LinkTypeCopy, Fallback: true, Message: "created a copy because symlink and junction failed"}, nil
	} else {
		return LinkResult{}, err
	}
}
