package core

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MaxSkillNameLen is the Agent Skills limit for the name field.
const MaxSkillNameLen = 64

// windowsReserved are device names that Windows rejects or aliases.
var windowsReserved = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// ValidateSkillName checks name against the Agent Skills name rules and
// rejects values that are unsafe as a single path segment.
//
// A name is 1 to 64 characters and must already be NFKC. It may contain
// lowercase letters, digits, and hyphens, and may not start or end with a
// hyphen or contain "--". Windows reserved device names are rejected.
// A name that changes under NFKC is rejected so the directory name is the
// string that was validated.
func ValidateSkillName(name string) error {
	if norm.NFKC.String(name) != name {
		return fmt.Errorf("%w: name is not NFKC", ErrInvalidName)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if name != strings.TrimSpace(name) || strings.HasSuffix(name, ".") {
		return fmt.Errorf("%w: leading or trailing space or dot", ErrInvalidName)
	}
	if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) || filepathIsAbs(name) {
		return fmt.Errorf("%w: name must be a single path segment", ErrInvalidName)
	}
	runes := []rune(name)
	if len(runes) > MaxSkillNameLen {
		return fmt.Errorf("%w: name exceeds %d characters", ErrInvalidName, MaxSkillNameLen)
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("%w: name must be lowercase", ErrInvalidName)
	}
	prevHyphen := false
	for i, r := range runes {
		switch {
		case r == '-':
			if i == 0 || prevHyphen {
				return fmt.Errorf("%w: hyphen placement", ErrInvalidName)
			}
			prevHyphen = true
		case unicode.IsDigit(r):
			prevHyphen = false
		case unicode.IsLetter(r) && unicode.IsLower(r):
			prevHyphen = false
		default:
			return fmt.Errorf("%w: character %q is not allowed", ErrInvalidName, string(r))
		}
	}
	if prevHyphen {
		return fmt.Errorf("%w: name must not end with a hyphen", ErrInvalidName)
	}
	if _, ok := windowsReserved[name]; ok {
		return fmt.Errorf("%w: %q is a reserved device name", ErrInvalidName, name)
	}
	return nil
}

func filepathIsAbs(name string) bool {
	if name == "" {
		return false
	}
	if name[0] == '/' || name[0] == '\\' {
		return true
	}
	// Windows drive prefix, rejected on every OS so a name cannot smuggle one.
	return len(name) >= 2 && name[1] == ':'
}
