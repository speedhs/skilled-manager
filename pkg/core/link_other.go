//go:build !windows

package core

func linkFallback(_ fileSystem, _, _ string, err error) (LinkResult, error) {
	return LinkResult{}, err
}
