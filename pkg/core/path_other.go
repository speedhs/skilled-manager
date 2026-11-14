//go:build !windows

package core

func volumeRoot(string) bool { return false }
