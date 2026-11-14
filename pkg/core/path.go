package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// unixVar matches $NAME.
var unixVar = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)`)

// winVar matches %NAME%.
var winVar = regexp.MustCompile(`^%([A-Za-z_][A-Za-z0-9_]*)%`)

// ExpandPath expands a leading ~ and environment variables.
//
// ${NAME:-default} uses default when NAME is unset or empty. $NAME, ${NAME},
// and %NAME% are errors when unset or empty, so a missing variable cannot
// become the filesystem root or an empty segment. The expanded path must
// not be empty or the filesystem root.
func ExpandPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: path is empty", ErrInvalidPath)
	}
	expanded, err := expandVariables(path, 0)
	if err != nil {
		return "", err
	}
	expanded, err = expandTilde(expanded)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(expanded) == "" {
		return "", fmt.Errorf("%w: expansion of %q is empty", ErrInvalidPath, path)
	}
	cleaned := filepath.Clean(expanded)
	if isFilesystemRoot(cleaned) {
		return "", fmt.Errorf("%w: expansion of %q is the filesystem root", ErrInvalidPath, path)
	}
	return cleaned, nil
}

func expandVariables(path string, depth int) (string, error) {
	if depth > 8 {
		return "", fmt.Errorf("%w: variable expansion is too deep", ErrInvalidPath)
	}
	var b strings.Builder
	for i := 0; i < len(path); {
		rest := path[i:]
		if name, def, hasDef, n, ok := matchBrace(rest); ok {
			val, found := lookupVar(name)
			if !found {
				if !hasDef {
					return "", fmt.Errorf("%w: variable %s is unset or empty", ErrInvalidPath, name)
				}
				val = def
			}
			next, err := expandVariables(val, depth+1)
			if err != nil {
				return "", err
			}
			b.WriteString(next)
			i += n
			continue
		}
		if m := unixVar.FindStringSubmatch(rest); m != nil {
			val, ok := lookupVar(m[1])
			if !ok {
				return "", fmt.Errorf("%w: variable %s is unset or empty", ErrInvalidPath, m[1])
			}
			b.WriteString(val)
			i += len(m[0])
			continue
		}
		if m := winVar.FindStringSubmatch(rest); m != nil {
			val, ok := lookupVar(m[1])
			if !ok {
				return "", fmt.Errorf("%w: variable %s is unset or empty", ErrInvalidPath, m[1])
			}
			b.WriteString(val)
			i += len(m[0])
			continue
		}
		b.WriteByte(path[i])
		i++
	}
	return b.String(), nil
}

// matchBrace parses ${NAME} or ${NAME:-default} at the start of s.
// Defaults may contain nested ${...} forms. n is the number of bytes consumed.
func matchBrace(s string) (name, def string, hasDef bool, n int, ok bool) {
	if !strings.HasPrefix(s, "${") {
		return "", "", false, 0, false
	}
	i := 2
	for i < len(s) && isNameByte(s[i]) {
		i++
	}
	if i == 2 {
		return "", "", false, 0, false
	}
	name = s[2:i]
	if strings.HasPrefix(s[i:], ":-") {
		j := i + 2
		depth := 1
		for j < len(s) && depth > 0 {
			if strings.HasPrefix(s[j:], "${") {
				depth++
				j += 2
				continue
			}
			if s[j] == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
			j++
		}
		if depth != 0 {
			return "", "", false, 0, false
		}
		return name, s[i+2 : j], true, j + 1, true
	}
	if i < len(s) && s[i] == '}' {
		return name, "", false, i + 1, true
	}
	return "", "", false, 0, false
}

func isNameByte(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func lookupVar(name string) (string, bool) {
	val, ok := os.LookupEnv(name)
	if !ok || val == "" {
		return "", false
	}
	return val, true
}

func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		if strings.HasPrefix(path, "~") {
			return "", fmt.Errorf("%w: ~user expansion is not supported", ErrInvalidPath)
		}
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("%w: home directory is unavailable", ErrInvalidPath)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func isFilesystemRoot(path string) bool {
	if path == "/" || path == `\` {
		return true
	}
	return volumeRoot(path)
}

// CanonicalizePath returns an absolute, cleaned form of path.
// It does not follow symlinks.
func CanonicalizePath(path string) (string, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidPath, err)
	}
	return filepath.Clean(abs), nil
}

// ensureWithin reports an error when path is not root or a descendant of root.
// Comparison is lexical after Abs and Clean. Symlinks are not followed.
func ensureWithin(root, path string) error {
	if !dirContains(root, path) {
		return fmt.Errorf("%w: %s is outside %s", ErrInvalidPath, path, root)
	}
	return nil
}

func dirContains(root, path string) bool {
	r, err1 := filepath.Abs(root)
	p, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	r = filepath.Clean(r)
	p = filepath.Clean(p)
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	aa = filepath.Clean(aa)
	bb = filepath.Clean(bb)
	return pathsEqual(aa, bb)
}

func resolveLink(linkPath, target string) string {
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	return filepath.Clean(target)
}

func linksTo(linkPath, linkTarget, want string) bool {
	return samePath(resolveLink(linkPath, linkTarget), want)
}

// skillDir joins a validated skill name onto canonical.
func skillDir(canonical, name string) (string, error) {
	if err := ValidateSkillName(name); err != nil {
		return "", err
	}
	dir := filepath.Join(canonical, name)
	if err := ensureWithin(canonical, dir); err != nil {
		return "", err
	}
	return dir, nil
}

// hermeticGuard refuses writes into the real home directory when tests set
// SKILLS_TEST_MODE=1 and SKILLS_ORIGINAL_HOME.
func hermeticGuard(path string) error {
	if os.Getenv("SKILLS_TEST_MODE") != "1" {
		return nil
	}
	orig := os.Getenv("SKILLS_ORIGINAL_HOME")
	if orig == "" || path == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !dirContains(orig, abs) {
		return nil
	}
	if home, herr := os.UserHomeDir(); herr == nil && !samePath(home, orig) && dirContains(home, abs) {
		return nil
	}
	if dirContains(os.TempDir(), abs) {
		return nil
	}
	return fmt.Errorf("%w: refusing to touch the real home directory (%s)", ErrInvalidPath, abs)
}
