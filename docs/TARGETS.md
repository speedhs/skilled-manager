# Target paths

Checked on **2026-11-14** against the upstream pages linked below. The defaults live in `pkg/core/targets/catalog.json` (`version` 1, `checked` `2026-11-14`). Changing a path is a catalog edit, not a code edit. An existing `targets.json` is not rewritten; `skills doctor` warns when a built-in id still points at a different path.

| Tool | Default path | Mode | Format | Source |
| --- | --- | --- | --- | --- |
| Claude Code | `~/.claude/skills/<name>/SKILL.md` | symlink | skill | [Claude Code skills](https://code.claude.com/docs/en/skills/) |
| Codex CLI | `~/.agents/skills` | symlink | skill | [Build skills](https://learn.chatgpt.com/docs/build-skills) |
| OpenCode | `${XDG_CONFIG_HOME:-~/.config}/opencode/skills` | symlink | skill | [OpenCode skills](https://opencode.ai/docs/skills/) |
| Gemini CLI | `~/.gemini/skills` | symlink | skill | [Creating Agent Skills](https://geminicli.com/docs/cli/creating-skills/) |

## Claude Code

The skills page lists the personal location as `~/.claude/skills/<skill>/SKILL.md` and the project location as `.claude/skills/<skill>/SKILL.md`. This tool deploys the personal location. Claude Code also reserves the folder name `synced` under `~/.claude/skills/` for skills downloaded from claude.ai. Skills Manager does not treat `synced` as a special name; do not publish a skill with that directory name if you use Claude Code's account sync.

## Codex CLI

The user scope is `$HOME/.agents/skills`. Repository scopes are `.agents/skills` from the working directory up to the repository root. `~/.codex/config.toml` is configuration, not the skill directory. `~/.codex/skills` is not a second default.

## OpenCode

The discovery table lists the global source as `~/.config/opencode/skills` (plural). The same page's config docs put user settings at `~/.config/opencode/opencode.json` on every platform, including macOS and Windows, rather than `~/Library/Application Support`. The catalog path uses `${XDG_CONFIG_HOME:-~/.config}` so an unset variable expands to that documented directory. If `XDG_CONFIG_HOME` is set, the deployment follows it. If a future OpenCode build ignores that variable, set the target path explicitly. OpenCode also reads `~/.claude/skills` and `~/.agents/skills` for compatibility; this tool does not deploy a second copy there.

Project skills live in `.opencode/skills/` and are out of scope for the default target.

## Gemini CLI

User skills are `~/.gemini/skills/` (with `~/.agents/skills/` as an alias) and workspace skills are `.gemini/skills/`. The default target is the user directory `~/.gemini/skills`. Deploying the alias as well would duplicate the skill, so it is not a second default.

Gemini's older custom commands are TOML files. That is a different feature from Agent Skills. Format `gemini-toml` is still accepted on a target and writes one TOML file from the description and body. It drops `scripts/`, `references/`, `assets/`, and frontmatter fields other than name and description. A script-only change in the canonical skill does not mark a TOML target as drifted, because the TOML bytes do not include those files. Prefer format `skill`.

## What "current" does not include

These pages also describe project, enterprise, and plugin locations. The built-in targets are the per-user directories only. Add a target in Settings or `targets.json` for a project directory.
