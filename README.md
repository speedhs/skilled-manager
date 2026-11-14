# Skills Manager

Skills Manager keeps Agent Skills (`SKILL.md` folders) in one directory and deploys them to Claude Code, Codex CLI, OpenCode, and Gemini CLI. The CLI is static Go. The desktop window uses Fyne. Library code is `pkg/core` and does not import Fyne.

The module path is `github.com/speedhs/skilled-manager`.

## Install the CLI

From a checkout:

```bash
CGO_ENABLED=0 go install ./cmd/skills
```

After this commit is on the default branch:

```bash
CGO_ENABLED=0 go install github.com/speedhs/skilled-manager/cmd/skills@latest
```

`skills version` prints the version, commit, and build date. `skills help` lists commands. Exit codes are 0 success, 1 error, 2 usage, 3 conflict, and 4 partial success.

Release archives are built by GoReleaser for Linux, macOS, Windows, and FreeBSD. See [docs/RELEASE.md](docs/RELEASE.md). GUI builds are unsigned until signing certificates exist. Read `packaging/UNSIGNED.txt` before the first launch.

## Build

Go 1.25 or newer. The GUI also needs a C compiler and, on Linux, OpenGL and X11 headers.

```bash
make build-cli
make test
make build-gui   # CGO
```

## Library

```go
mgr, err := core.NewManager(
    core.WithCanonicalDir(canonicalDir),
    core.WithConfigDir(configDir),
)
skill, err := core.CreateSkill(mgr.CanonicalDir, "code-reviewer", "Reviews pull requests", "# Review\n")
_, err = core.ListSkills(mgr.CanonicalDir)
_, err = mgr.Enable(ctx, "code-reviewer", "claude")
status, err := mgr.Status(ctx, "code-reviewer", "claude")
_, err = mgr.Sync(ctx)
```

`TestReadmeSnippet` in `pkg/core/readme_example_test.go` compiles those calls. Long operations take `context.Context`. Dry-run is `core.WithDryRun`.

## Targets

Checked on 2026-11-14. Details and links are in [docs/TARGETS.md](docs/TARGETS.md).

| Tool | Path | Mode |
| --- | --- | --- |
| Claude Code | `~/.claude/skills` | symlink |
| Codex CLI | `~/.agents/skills` | symlink |
| OpenCode | `${XDG_CONFIG_HOME:-~/.config}/opencode/skills` | symlink |
| Gemini CLI | `~/.gemini/skills` | symlink |

`gemini-toml` is optional and lossy: it keeps the description and body and drops scripts, references, assets, and extra frontmatter. An existing `targets.json` is not rewritten. `skills doctor` warns when a built-in id differs from the catalog.

## Commands

```bash
skills list
skills new my-helper "Assists with writing unit tests"
skills enable my-helper claude
skills --yes disable my-helper claude
skills sync
skills import --scan
skills --yes import --all
skills export my-skills.zip
skills doctor
skills backups
skills restore <id>
skills completion bash
```

`--json` is accepted by `list`, `status`, `sync`, `targets`, `doctor`, `import --scan`, and `version`. `--dry-run` performs the checks and writes nothing. `--yes` is required for disable, restore, and `import --all` when stdin is not a terminal. `NO_COLOR` is honored because the CLI prints no color. `--verbose` logs to stderr.

## Doctor

`skills doctor` checks the canonical directory, the config directory, symlink creation, each target path, broken links, and state entries that no longer match. A failing check is exit code 1. The `fix` field is the next command or filesystem change to try.

| Check | What it usually means |
| --- | --- |
| canonical or config `fail` | The directory is missing or not writable. Pass `--canonical` or `--config`. |
| symlink `fail` | This environment cannot create a symlink. On Windows the enable path falls back to a junction, then a copy, and records which one it used. |
| target `fail` | The path did not expand, or a variable was empty. Set the variable or edit the target. |
| conflict | The destination is not one this tool created, or both copies changed. Nothing is overwritten. Move the foreign file aside yourself, then enable again. |
| lock `fail` | Another `skills` process or the GUI holds `manager.lock`. |

## Limits

- A path is deleted only when it is a symlink to the canonical skill, or a copy recorded in `state.json` with a matching inode or content hash. Disable of anything else returns a conflict and leaves the file.
- Sync updates a deployment only when the canonical skill changed and the deployed bytes did not. It does not pick a side when both changed.
- Windows enable tries a symlink, then a directory junction, then a copy. That fallback is implemented and covered by cross-compiled code. It has not been executed on a Windows machine in this workspace. CI includes a Windows runner.
- macOS volumes are case-insensitive. The name check rejects a second skill whose name differs only by case. That check was not run on an APFS volume here.
- Zip import rolls back the skill that failed. Skills earlier in the same zip stay imported.
- If saving state fails after a drifted copy was replaced, the new bytes remain and a backup is kept. The next status is a conflict, so sync will not overwrite it again.
- The tool never runs scripts inside a skill and never opens a network connection.
- There is no automatic update check.

## Development

```bash
make tools
make lint
make test-race
make fuzz
make cover
make release-snapshot
```

[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) describes the status machine. [CONTRIBUTING.md](CONTRIBUTING.md) is the contributor guide. [SECURITY.md](SECURITY.md) is the disclosure process.
