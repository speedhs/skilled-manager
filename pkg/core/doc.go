// Package core manages Agent Skills on disk.
//
// A Manager keeps one canonical directory of SKILL.md folders and deploys
// them to tool-specific directories. Deployment creates a symlink or, when
// the target is configured for copies or Gemini TOML commands, a file the
// manager recorded in state.json.
//
// Paths the manager did not create are conflicts. Enable, disable, sync,
// import, and restore refuse to overwrite or delete them.
//
// Skill directories may contain a scripts/ folder. This package never
// executes those files. Callers must not execute them either; skill text
// and scripts are untrusted data.
//
// Mutations are single-writer. Each mutating method takes an in-process
// lock and an exclusive lock file in the config directory so a second
// process cannot interleave a change. Status may run alongside other
// status calls. A status result is a point-in-time view.
//
// Long operations accept a context. Cancellation is checked between
// skills and targets. A cancelled call does not roll back work that
// already committed, except where a single deployment failed before its
// state record was saved.
package core
