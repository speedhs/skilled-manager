package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ExpandPath expands the tilde (~) to the user's home directory
// and expands any environment variables like $HOME, ${VAR}, or Windows %VAR%.
func ExpandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand environment variables
	path = os.ExpandEnv(path)
	if runtime.GOOS == "windows" {
		// Also support Windows %VAR% syntax if not already handled
		path = expandWindowsEnv(path)
	}

	// Expand leading tilde
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Clean(home)
		}
	} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Clean(filepath.Join(home, path[2:]))
		}
	}

	return filepath.Clean(path)
}

// expandWindowsEnv expands %VAR% in paths on Windows
func expandWindowsEnv(path string) string {
	for {
		start := strings.Index(path, "%")
		if start == -1 {
			break
		}
		end := strings.Index(path[start+1:], "%")
		if end == -1 {
			break
		}
		end = start + 1 + end
		varName := path[start+1 : end]
		val := os.Getenv(varName)
		path = path[:start] + val + path[end+1:]
	}
	return path
}

// CanonicalizePath returns an absolute, evaluated (if symlink resolved) clean path.
func CanonicalizePath(path string) (string, error) {
	expanded := ExpandPath(path)
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %q: %w", path, err)
	}
	return filepath.Clean(abs), nil
}
