# Production plan

Ordered work. Each item is done when its acceptance criteria hold and `go test -race` is green for the packages that item touches. Product decisions are recorded here and implemented; they are not left open.

## Decisions

1. **Module path is `github.com/speedhs/skilled-manager`.** That is the Git remote (`speedhs/skilled-manager`). The README path `github.com/skilled-manager/skills-manager` does not match any remote, so `go install` of it cannot work. Install command: `go install github.com/speedhs/skilled-manager/cmd/skills@latest` after this commit is on the default branch. Local verification is `go install ./cmd/skills` from a clean module cache; `@latest` itself needs the commit on the remote.
2. **Gemini's default target is a skill directory, `~/.gemini/skills`, mode `symlink`, format `skill`.** TOML commands stay available as format `gemini-toml` for people who want slash commands. TOML conversion is lossy and is not the default. Existing `targets.json` files are not rewritten; `skills doctor` warns when a built-in id still points at a stale catalog path.
3. **Codex default is `~/.agents/skills`.** `~/.codex/skills` is documented as legacy, not deployed twice.
4. **OpenCode default is `${XDG_CONFIG_HOME:-~/.config}/opencode/skills`.** Unset `XDG_CONFIG_HOME` uses `~/.config` on Linux, macOS, and Windows, which matches OpenCode's xdg-basedir behavior. A variable with no default that is unset or empty is an error.
5. **Sync never picks a winner.** Only "canonical changed, deployed bytes unchanged" is auto-updated. Target edits and both-sides edits are `conflict`. Explicit enable of a `conflict` is refused. Drifted (canonical-only) updates take a backup first.
6. **Semver stays v0** until a later major. `apidiff` runs in CI only when the latest tag is v1 or newer.
7. **No update checker.** An update check needs the network. The CLI and library must not open network connections. The idea is on the roadmap.
8. **Desktop builds are unsigned** until a Developer ID and a Windows certificate exist. Release notes and the About dialog say so, and `docs/RELEASE.md` lists the first-run click-through.
9. **Oldest supported Go is 1.25.** `fsnotify` only needs 1.23. `golang.org/x/text` v0.39.0, which fixes GO-2026-5970, requires Go 1.25. CI tests 1.25 and the latest stable.

## Phase 0 — identity and targets

- One module path in `go.mod`, imports, README, GoReleaser, and workflows.
- `pkg/core/targets/catalog.json` is the versioned default target list. `targets.json` accepts the legacy array and `{version, targets}`.
- Frontmatter follows the Agent Skills spec: name shape, 64/1024/500 limits, known fields, duplicate keys rejected, unknown fields preserved on round-trip.
- Acceptance: `go install ./cmd/skills` produces a binary; catalog tests pin the documented paths; frontmatter tests reject spec violations and round-trip an unknown field.

## Phase 1 — safety

- `ValidateSkillName` on every name that becomes a path. Reject separators, `..`, absolute paths, Windows reserved names, trailing dots/spaces, NFKC lookalikes, and case-collisions with an existing folder.
- Delete only a symlink whose cleaned target is the canonical skill, or a non-symlink whose recorded content hash matches and, when an inode was stored, whose inode still matches. Open with `O_NOFOLLOW` and compare inode before unlinking. Never walk a symlink or junction. Do not delete a copy whose bytes changed. Do not delete a deployment when its backup cannot be kept.
- Create with `O_EXCL` / `os.Symlink` failure. Do not delete first.
- Zip export and import: reject zip-slip, symlink entries, absolute names, odd modes, and bombs (count, uncompressed size, ratio).
- Import copies to a staging directory, links back, then removes the source. Any failure restores the source and removes the staging copy.
- `state.json` and `targets.json`: temp file, fsync, rename, schema version. Corrupt state is renamed aside and replaced with an empty state plus a warning.
- File lock `manager.lock` around mutations. In-process mutex around the same critical section.
- Watcher: debounce, coalesce storms, surface overflow, `sync.Once` close, goleak test.
- ExpandPath fails on unresolved or empty variables and refuses to return `/` or an empty path.
- Hash is a streaming content hash, ignores `.git`, `.DS_Store`, `Thumbs.db`, and does not follow symlinks.
- Backups under the config dir before replacing a managed copy or TOML, with retention. `Restore` refuses unmanaged paths.
- Acceptance: adversarial tests (symlink to a canary home, replaced directory, symlink loop, zip-slip, zip bomb, partial import, both-sides sync, unset env var).

## Phase 2 — tests

- `pkg/core` coverage at least 85% of statements.
- `TestMain` sets `HOME`/`USERPROFILE` to a temp dir and `SKILLS_TEST_MODE=1`. `NewManager` refuses paths inside `SKILLS_ORIGINAL_HOME`.
- Fuzz tests for frontmatter, path expansion, names, zip, and TOML, with committed seeds.
- Property-style tests: enable then disable restores the tree; sync twice is a no-op; export then import round-trips.
- Fault-injecting `FS` for `ENOSPC` on state save rolls the deployment back.
- `testscript` covers every CLI command, including exit codes.
- Fyne tests for the list, editor validation, and matrix status text.
- CI: Linux, macOS (arm64 and amd64), Windows; Go 1.25 and latest; `-race` where the OS supports it. GUI tests run where a C compiler and OpenGL headers exist.

## Phase 3 — CLI

- Exit codes: 0 ok, 1 error, 2 usage, 3 conflict, 4 partial.
- `--json` (schema version 1) on list, status, sync, targets, import --scan, doctor.
- `--dry-run` on mutating commands. `--yes` or a TTY prompt for disable, import, sync, and restore. `--verbose` / `--quiet`. `NO_COLOR` disables ANSI.
- `skills doctor`, `skills version`, `skills completion` (bash, zsh, fish, pwsh), and a man page.
- Library logging is `*slog.Logger`, default discard. No `log.Fatal` or `os.Exit` in `pkg/core`.

## Phase 4 — library API

- Exported identifiers documented. Long operations take `context.Context`. Errors wrap `ErrConflict`, `ErrNotFound`, `ErrInvalidName`, `ErrInvalidPath`, `ErrLocked`, `ErrExists`.
- Example tests compile, and the README sample matches them.
- CI script fails if `pkg/core` depends on Fyne.
- `scripts/apidiff.sh` is a no-op on v0 and a gate on v1.

## Phase 5 — GUI

- Mutations run in goroutines. Results return through `fyne.Do`. Buttons disable while busy.
- Errors use a dialog with the summary and a details expander.
- Disable and sync ask for confirmation and name the paths.
- Editor: dirty flag, close guard, external change prompt (reload or keep), atomic save, undo/redo, refuse files over 1 MiB.
- Matrix rows virtualized, status as text plus color, search debounced.
- Persist window size and last skill. Light/dark toggle. Settings show the expanded path and warn on built-in targets.

## Phase 6 — supply chain

- `govulncheck`, `gosec`, `staticcheck` in CI.
- Dependabot for Go modules and GitHub Actions.
- Actions pinned by SHA.
- GoReleaser: `-trimpath`, version ldflags, Syft SBOM, cosign signature of the checksum file on tag builds (keyless OIDC).
- `SECURITY.md`. A test fails if `cmd/skills` depends on `net/http`.

## Phase 7 — release

- GoReleaser archives for linux, darwin, windows, freebsd (amd64 and arm64, plus the existing extra arches that cross-compile).
- GUI packaging workflow stays, with a `.desktop` file and icon, and unsigned-build labeling.
- nfpm deb/rpm manifests. Homebrew and Scoop formulas generated with upload skipped until those repositories exist (`docs/RELEASE.md`).
- `skills version` and About show version, commit, and date.
- Pull requests run `goreleaser release --snapshot --clean --skip=publish,sign`.

## Phase 8 — docs and hygiene

- README install line, target matrix, doctor troubleshooting, limitations.
- `docs/ARCHITECTURE.md`, `docs/TARGETS.md`, `docs/RELEASE.md`, `CHANGELOG.md`, `CODE_OF_CONDUCT.md`.
- Makefile: `lint`, `test`, `test-race`, `fuzz`, `cover`, `build-cli`, `build-gui`, `release-snapshot`, `tools`.
- `.golangci.yml`, `.editorconfig`.

## Definition of done mapping

The checklist in the task is met when the phase acceptance criteria above are true, except items that need machines or certificates this workspace does not have. Those are listed in the final status as deferred, not marked done.
