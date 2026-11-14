# Changelog

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow semantic versioning, and this module stays at `v0` until the `pkg/core` API is frozen.

## [Unreleased]

### Security

- Refuse to delete or overwrite a path unless it is a symlink to the canonical skill or a copy recorded in `state.json` with matching identity.
- Reject unsafe skill names, unresolved path variables, zip-slip, and zip bombs.
- Write `state.json` atomically, quarantine a corrupt ledger, and lock mutations across processes.
- Do not execute skill scripts and do not link a network client into the CLI.

### Changed

- Module path is `github.com/speedhs/skilled-manager`.
- Default targets are the skill directories documented on 2026-11-14. Gemini TOML is an optional lossy format, not the default.
- Sync updates a deployment only when the canonical skill changed and the deployed bytes did not. Other differences are conflicts.
- CLI exit codes, `--json`, `--dry-run`, and `--yes` are stable. See `skills help`.

### Added

- `skills doctor`, `skills version`, `skills restore`, shell completions, and a manual page.
- Backups before replacing a managed copy, with retention.
- Version, commit, and build date in the CLI and the GUI About dialog.
