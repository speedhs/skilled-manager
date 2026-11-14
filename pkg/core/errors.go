package core

import "errors"

// Sentinel errors. Callers should use errors.Is.
var (
	// ErrConflict means the path exists and this tool did not create it,
	// or both the canonical skill and the deployed copy changed.
	ErrConflict = errors.New("conflict")

	// ErrNotFound means the skill, target, or backup does not exist.
	ErrNotFound = errors.New("not found")

	// ErrInvalidName means a skill name failed validation.
	ErrInvalidName = errors.New("invalid skill name")

	// ErrInvalidPath means a path is empty, escapes its root, or a
	// variable in it did not expand.
	ErrInvalidPath = errors.New("invalid path")

	// ErrLocked means another process holds the manager lock.
	ErrLocked = errors.New("skills manager is locked by another process")

	// ErrExists means the canonical skill or destination already exists.
	ErrExists = errors.New("already exists")

	// ErrCorrupt means a state file could not be parsed.
	ErrCorrupt = errors.New("corrupt state")

	// ErrInvalidFrontmatter means SKILL.md metadata failed validation.
	ErrInvalidFrontmatter = errors.New("invalid frontmatter")
)
