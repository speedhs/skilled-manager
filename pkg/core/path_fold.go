//go:build windows || darwin

package core

import "strings"

func pathsEqual(a, b string) bool { return strings.EqualFold(a, b) }
