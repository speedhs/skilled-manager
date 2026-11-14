//go:build windows

package core

import "path/filepath"

func volumeRoot(path string) bool {
	vol := filepath.VolumeName(path)
	return vol != "" && filepath.Clean(path) == vol+`\`
}
