//go:build !windows && !darwin

package core

func pathsEqual(a, b string) bool { return a == b }
