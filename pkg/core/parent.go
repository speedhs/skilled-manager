package core

import "path/filepath"

func splitParent(path string) (string, string) {
	return filepath.Dir(path), filepath.Base(path)
}
