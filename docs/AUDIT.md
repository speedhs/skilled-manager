# Skills Manager audit

Audited at commit `ac6263e` on 2026-11-14. Commands attempted: `go vet ./...` (GUI package fails to compile here because Wayland headers are not installed; `pkg/core` and `cmd/skills` vet clean once the GUI package is excluded), `CGO_ENABLED=0 go build -o /tmp/skills ./cmd/skills` (succeeds), `go test -race ./pkg/core/...` (passes on the pre-fix tree). `staticcheck` was not installed in the environment at audit time.

Severity is about what the tool can do to a user's files. Data-loss outranks everything else.

## Data-loss

1. **Skill names are path-capable.** `CreateSkill` trims the name and joins it onto the canonical directory (`pkg/core/skill.go:104-119`). `..`, absolute paths, and separators escape that directory. Import replaces the folder name with frontmatter `name` (`pkg/core/import.go:113-115`) and then joins that string (`pkg/core/import.go:134`). Enable and status do the same (`pkg/core/enable.go:27`, `pkg/core/status.go:46`). A `SKILL.md` whose `name` is `../../.ssh` is enough.
2. **Disable deletes a symlink it could not read.** In `pkg/core/enable.go:183-197`, a `Readlink` error skips the "points at canonical" check and still calls `os.Remove`.
3. **Tracked copies are removed with `os.RemoveAll` and no identity check** (`pkg/core/enable.go:201-213`). Ownership is "a row exists in `state.json`". Replacing that directory with another object between the check and the delete is not detected. Windows junctions are not treated as reparse points.
4. **Enable is check-then-act.** Status is sampled (`pkg/core/enable.go:36-43`), then Gemini `os.WriteFile` truncates whatever is at the destination (`pkg/core/enable.go:63`). Symlink mode removes the destination before `os.Symlink` (`pkg/core/enable.go:87-96`). A file created after the status check is overwritten or deleted.
5. **Import is not transactional** (`pkg/core/import.go:159-196`). The source is removed before link-back succeeds. The cross-device path copies and then `RemoveAll`s the source even if the copy is incomplete. The error path "imported to canonical, but failed to link back" leaves the original location empty.
6. **`state.json` is written with `os.WriteFile`** (`pkg/core/state.go:75`). A crash mid-write corrupts the only record of which paths the tool is allowed to delete. There is no schema version, and a corrupt file makes `NewManager` fail (`pkg/core/state.go:47-49`) instead of quarantining the file and rebuilding.
7. **No cross-process lock.** The CLI and the GUI, or two CLI processes, can interleave enable and disable. `Manager.mu` is not held across `Enable` (`pkg/core/manager.go:19`, `pkg/core/enable.go:21`).
8. **Unset environment variables expand to empty** (`pkg/core/path.go:19`, `pkg/core/path.go:42-57`). A target path of `$MISSING/skills` becomes `skills` (relative) or, with a leading slash in the remainder, a path the user did not configure. `%VAR%` does the same on Windows.
9. **Sync overwrites the target whenever hashes differ** (`pkg/core/status.go:163-165`, `pkg/core/enable.go:245-247`). It cannot tell "canonical edited" from "user edited the copy" from "both edited", and it picks canonical. There is no backup and no dry-run.
10. **Copy and zip follow symlinks into file contents.** `CopyFile` uses `os.Open` (`pkg/core/link.go:134`). `HashFile` does too (`pkg/core/hash.go:21`). Export walks and opens the same way (`pkg/core/export.go:69-75`). A symlink inside a skill can pull an arbitrary file into a copy or a zip. There is no zip-slip guard because there is no zip import, and export does not reject absolute names, symlink entries, or oversized archives.
11. **`RemoveSkill` joins the raw name and `RemoveAll`s it** (`pkg/core/skill.go:186-192`).

## Correctness

12. **CLI flags are never parsed.** `cmd/skills/main.go:45-48` builds a `FlagSet` and never calls `Parse`. `-canonical` and `-config` are documented and inert, so the CLI always uses the real home directory.
13. **Module path does not match the Git remote.** `go.mod` and the README say `github.com/skilled-manager/skills-manager`. `origin` is `https://github.com/speedhs/skilled-manager.git`. `go install` of the README module path cannot resolve this repository. GoReleaser's `main.version` ldflags (`/.goreleaser.yaml:41-43`) have no matching variables in `cmd/skills/main.go`.
14. **Default target paths are hardcoded and stale** (`pkg/core/config.go:33-63`). Checked against upstream docs on 2026-11-14 (see `docs/TARGETS.md` once written):
    - Claude Code `~/.claude/skills` is still correct.
    - Codex CLI's current user directory is `~/.agents/skills`. `~/.codex/skills` is legacy.
    - OpenCode's directory is `skills` (plural), under `$XDG_CONFIG_HOME` or `~/.config` on every OS, not `~/.config/opencode/skill`.
    - Gemini CLI discovers skills at `~/.gemini/skills`. `~/.gemini/commands/*.toml` is a separate slash-command feature. Converting a skill folder to TOML drops scripts, references, assets, and extra frontmatter.
15. **Frontmatter validation does not implement the Agent Skills spec** (`pkg/core/frontmatter.go:92-110`). Missing: length limits, character set, hyphen rules, exact directory match (the check uses `EqualFold`, line 105), duplicate-key rejection, and typed checks for `compatibility` and `metadata`. Unknown fields must round-trip; `yaml:",inline"` is not covered by tests.
16. **Hashing has no ignore list and is sensitive to symlinks** (`pkg/core/hash.go:52-77`). `.git`, `.DS_Store`, and `Thumbs.db` change the hash. Metadata is already ignored, which is right; junk files are not.
17. **Watcher drops overflow and other errors** (`pkg/core/watcher.go:162-166`). `Close` closes `stopCh` unconditionally (`pkg/core/watcher.go:56-58`), so a second close panics. Debounce exists, but rename-over-write and shutdown leaks are untested.
18. **Enable-all and import-all ignore partial failure** (`cmd/skills/main.go:177-188`, `cmd/skills/main.go:275-281`). The process exits 0 when some targets failed.
19. **`PathsReferToSameLocation` is case-sensitive except on Windows** (`pkg/core/link.go:193-196`). macOS default volumes are case-insensitive.
20. **Import scan follows directory symlinks** via `os.Stat` (`pkg/core/import.go:28`) and can treat a linked folder as a real skill.
21. **Windows symlink → junction → copy is untested on a Windows runner.** CI (`/.github/workflows/ci.yml:20-33`) does not cover spaces, unicode, long paths, or a non-elevated junction fallback. `mklink` is invoked through `cmd /c` (`pkg/core/link.go:81`), which interprets `&`, `|`, and quotes.
22. **`go` version claim is inconsistent.** README says Go 1.21+. `go.mod` says `go 1.27.1`. `fsnotify` requires Go 1.23.

## Security

23. **CI has no `govulncheck`, `gosec`, or `staticcheck`.** Actions are pinned by tag (`actions/checkout@v4`, `actions/setup-go@v5`, `goreleaser/goreleaser-action@v5`, `softprops/action-gh-release@v2`), not by SHA. GoReleaser itself is `version: latest`.
24. **No `SECURITY.md`, no SBOM, no signed checksums, no provenance.**
25. **Nothing documents that `scripts/` is never executed.** The code does not execute them today. There is no dependency test proving the CLI does not import `net/http`.
26. **Export creates or truncates the output path the user typed** (`cmd/skills/main.go:296`) with no confirmation.

## UX

27. **Filesystem work runs on the UI thread.** Matrix enable/disable (`internal/ui/matrix.go:132-149`), create (`internal/ui/dialogs.go:60`), sync (`internal/ui/dialogs.go:324`), save (`internal/ui/editor.go:111`), and `Refresh` (`internal/ui/app.go:133-139`) all call `pkg/core` synchronously. The watcher marshals back with `fyne.Do` (`internal/ui/app.go:175`) and then blocks the UI thread inside `Refresh`.
28. **Disable does not show what will be removed** (`internal/ui/matrix.go:143`).
29. **The editor has no dirty flag, no external-change prompt, no undo, and no large-file limit** (`internal/ui/editor.go`). Saves use `os.WriteFile` (`pkg/core/skill.go:170`).
30. **The matrix builds every row up front** (`internal/ui/matrix.go:87-105`). Status text exists beside the color dot (`internal/ui/types.go:34-46`), so status is not color-only, but the rows are not virtualized and search is not debounced (`internal/ui/skill_list.go:34`).
31. **Window size, last selection, and theme are not persisted** (`internal/ui/app.go:33`). Settings do not show the expanded path or warn on built-in targets (`internal/ui/dialogs.go:276-307`).
32. **Scan and watcher failures are swallowed** (`pkg/core/import.go:33-35`, `pkg/core/watcher.go:162-166`).

## Maintainability

33. **Tests use a temp directory, which is necessary and not sufficient.** There is no guard that fails a test for touching the real home. GUI tests are behind `//go:build fyne_gui` (`internal/ui/ui_test.go:1`) and are not run by `make test` or CI.
34. **No fuzz tests, property tests, or filesystem fault injection.** Coverage is whatever `core_test.go` happens to hit. Status is not table-tested across mode × outcome.
35. **CI is one Go version and three OSes, race-only on `pkg/core`** (`.github/workflows/ci.yml`). No snapshot release on pull requests. No `golangci-lint` config, no `.editorconfig`.
36. **Public `pkg/core` API has no `context.Context`, no sentinel errors, and no runnable examples.** `os.Exit` is confined to `cmd/skills` (good). Several exported functions document the happy path only.
37. **Release metadata is incomplete.** No changelog, no architecture doc, no cited target matrix, no desktop entry, no Homebrew/Scoop/nfpm story, unsigned GUI builds with no first-run note.

## What is already sound

- A foreign directory is reported as `conflict`, and enable/disable refuse it, covered by `TestCollisionRefusal`.
- When `Readlink` succeeds, disable requires the symlink to resolve to the canonical skill (`pkg/core/enable.go:189-191`).
- `pkg/core` does not import Fyne.
- Core tests construct a manager with temp canonical and config directories (`pkg/core/core_test.go:14-45`).
- Directory hashes sort relative paths and use forward slashes (`pkg/core/hash.go:64-86`).
