# Skills Manager 🛠️

**Skills Manager** is a cross-platform native desktop application and CLI written in Go using [Fyne](https://fyne.io/) (v2.5+) that organizes and manages AI agent "skills" (`SKILL.md` folders) from a single canonical store and exposes them to multiple AI tools: **Claude Code**, **Codex CLI**, **OpenCode**, and **Gemini CLI**.

---

## 📑 Table of Contents
- [Architecture Overview](#architecture-overview)
- [Core Model & Storage](#core-model--storage)
- [Tool Target Matrix](#tool-target-matrix)
- [Features](#features)
- [Prerequisites & System Requirements](#prerequisites--system-requirements)
- [Quickstart & Build Steps](#quickstart--build-steps)
- [CLI Usage](#cli-usage)
- [GUI Walkthrough](#gui-walkthrough)
- [Safety & Collision Guarantees](#safety--collision-guarantees)
- [Assumptions & Path Verification](#assumptions--path-verification)
- [Testing & Quality Assurance](#testing--quality-assurance)

---

## Architecture Overview

```
skills-manager/
├── cmd/
│   ├── skills/                  # Command-line interface
│   │   └── main.go
│   └── skills-gui/              # Native Fyne GUI application
│       └── main.go
├── internal/
│   ├── core/                    # Pure Go domain logic (Zero Fyne imports, 100% tested)
│   │   ├── config.go            # Target configurations & defaults
│   │   ├── enable.go            # Enable, Disable, and Sync operations
│   │   ├── export.go            # ZIP export of skills
│   │   ├── frontmatter.go       # YAML frontmatter parsing, validation & serialization
│   │   ├── gemini.go            # Gemini CLI TOML conversion
│   │   ├── hash.go              # Deterministic directory & file hashing for drift detection
│   │   ├── import.go            # Scanning & importing existing target skills
│   │   ├── link.go              # Cross-platform symlink/junction/copy fallback
│   │   ├── manager.go           # Central Manager coordinator
│   │   ├── path.go              # Path expansion (~, $ENV, %VAR%)
│   │   ├── skill.go             # Canonical skill CRUD & parsing
│   │   ├── state.go             # State tracking (~/.agent-skills-manager/state.json)
│   │   ├── status.go            # Cell status evaluation (enabled/disabled/conflict/drifted/broken)
│   │   ├── watcher.go           # fsnotify watcher with debounced event dispatch
│   │   ├── core_test.go         # Comprehensive unit tests
│   │   └── gemini_test.go       # Gemini TOML unit tests
│   └── ui/                      # Fyne UI layer (calls only core)
│       ├── app.go               # Main window layout, split views, watcher integration
│       ├── dialogs.go           # New Skill, Import Existing, Settings, Sync Report dialogs
│       ├── editor.go            # Multi-line SKILL.md editor with frontmatter validation
│       ├── matrix.go            # Skills x Tools interactive matrix
│       ├── skill_list.go        # Searchable skill list pane
│       ├── toolbar.go           # Top action toolbar
│       ├── types.go             # UI constants, status color palette
│       └── ui_test.go           # UI component test suite
├── Makefile                     # Build & test automation
├── go.mod
├── go.sum
└── README.md
```

---

## Core Model & Storage

1. **Canonical Store**:
   - Location: `~/.agent-skills/<skill-name>/SKILL.md` (plus optional `scripts/`, `refs/`, etc.).
   - Format: `SKILL.md` with YAML frontmatter delimited by `---` and a Markdown body:
     ```markdown
     ---
     name: git-commit-helper
     description: Generates clean conventional commit messages
     ---
     # Git Commit Helper
     Instructions for writing commit messages...
     ```

2. **Configuration File**:
   - Location: `~/.agent-skills-manager/targets.json`
   - Created with sensible defaults if missing, and fully editable from the UI Settings dialog or CLI:
     ```json
     [
       {"id":"claude","label":"Claude Code","path":"~/.claude/skills","mode":"symlink","format":"skill"},
       {"id":"codex","label":"Codex CLI","path":"~/.codex/skills","mode":"symlink","format":"skill"},
       {"id":"opencode","label":"OpenCode","path":"~/.config/opencode/skill","mode":"symlink","format":"skill"},
       {"id":"gemini","label":"Gemini CLI","path":"~/.gemini/commands","mode":"copy","format":"gemini-toml"}
     ]
     ```

3. **State Tracker**:
   - Location: `~/.agent-skills-manager/state.json`
   - Tracks created copies, generated TOML files, content hashes, and Windows fallback modes to ensure safe deletion and drift detection.

---

## Tool Target Matrix

| Status | Dot / Badge | Description | Action Behavior |
| :--- | :---: | :--- | :--- |
| **`enabled`** | 🟢 Green | Active and synchronized with canonical store | Unchecking disables and removes symlink/copy |
| **`disabled`** | ⚪ Gray | Not installed in target directory | Checking enables skill for that target |
| **`drifted`** | 🟡 Yellow | Target copy or Gemini TOML differs from canonical `SKILL.md` | "Sync All" or re-checking updates copy |
| **`conflict`** | 🔴 Red | Target exists but was NOT created by Skills Manager | Collision refused; protected from overwrite/deletion |
| **`broken`** | 🟣 Purple | Symlink target is missing or dangling | "Sync All" repairs link if canonical exists |

---

## Features

- **Single Canonical Source**: Maintain skills in one directory (`~/.agent-skills`) and deploy anywhere.
- **Cross-Platform Symlinks & Fallbacks**:
  - Linux & macOS: Direct filesystem symlinks.
  - Windows: Attempts symlinks; automatically falls back to Directory Junctions (`mklink /J`) or Directory Copies when elevated privileges are not granted.
- **Gemini CLI Command Generation**: Converts `SKILL.md` (`description` + `prompt` body) into standard Gemini `.toml` command files automatically.
- **Drift Detection**: SHA-256 content hashing across directory trees and TOML files detects when canonical skills or targets change.
- **Collision Protection**: Never overwrites or deletes files or folders that were not created by Skills Manager.
- **Import Existing**: Scans target folders for existing real skill folders or `.toml` commands, moves them into the canonical store, and links them back in place.
- **Real-Time File Watcher**: `fsnotify` watches the canonical store and targets, with a 250ms debounce timer for responsive UI updates.
- **Full-Featured Editor**: In-app multi-line editor for `SKILL.md` with live YAML frontmatter validation.
- **ZIP Export**: Export all or selected skills with subfolders (`scripts/`, `refs/`) into a portable `.zip` archive.

---

## Prerequisites & System Requirements

- **Go**: Version 1.21 or higher (tested with Go 1.27+).
- **Linux GUI Requirements**:
  When compiling the desktop GUI on Linux (requires X11/Wayland and OpenGL libraries):
  ```bash
  sudo apt-get install -y libgl1-mesa-dev xorg-dev libwayland-dev
  ```
- **macOS / Windows**: Standard Go toolchain (no extra system libraries required).

---

## Quickstart & Build Steps

### 1. Run the Desktop GUI
```bash
go run ./cmd/skills-gui
```

### 2. Run the CLI
```bash
go run ./cmd/skills list
```

### 3. Build Binaries
```bash
# Build both CLI and GUI into ./bin
make all

# Or individually:
make build-cli
make build-gui
```

### 4. Run Unit Tests
```bash
make test
```

---

## CLI Usage

The `skills` CLI provides quick automation for terminal workflows:

```bash
# List all canonical skills and their deployment status per target
skills list

# Create a new canonical skill
skills new my-helper "Assists with writing unit tests"

# Enable a skill for all targets (or a specific target)
skills enable my-helper
skills enable my-helper gemini

# Disable a skill
skills disable my-helper claude

# Synchronize all drifted copies, TOML files, and broken links
skills sync

# Scan target directories for existing untracked skills
skills import --scan

# Import all discovered untracked skills into canonical store
skills import --all

# Export all canonical skills into a ZIP archive
skills export my-skills.zip

# List configured targets and their paths
skills targets
```

---

## GUI Walkthrough

```
+-----------------------------------------------------------------------------------------------+
| [New Skill]  [Import Existing]  [Sync All]  [Export Zip]  |  [Settings]  [Refresh]            |
+----------------------+------------------------------------------------------------------------+
| 🔍 Search skills...  | Tool Deployment Matrix                                                 |
+----------------------+------------------------------------------------------------------------+
| • git-commit-helper  | Skill          Claude Code       Codex CLI     OpenCode    Gemini CLI |
|   Generates commits  | git-commit...  [x] 🟢 Enabled   [x] 🟢 Enabled [ ] ⚪ Dis.. [x] 🟢 Enabled|
|                      | code-reviewer  [x] 🟢 Enabled   [ ] ⚪ Disab.. [x] 🟡 Drift [x] 🟢 Enabled|
| • code-reviewer      | untracked-ext  [ ] 🔴 Conflict  [ ] ⚪ Disab.. [ ] ⚪ Dis.. [ ] ⚪ Dis..  |
|   Reviews code       +------------------------------------------------------------------------+
|                      | SKILL.md Editor - git-commit-helper                [ Save SKILL.md ]   |
| • test-generator     | ✓ Frontmatter valid (name matches folder, description non-empty)       |
|   Writes Go tests    | ---------------------------------------------------------------------- |
|                      | ---                                                                    |
|                      | name: git-commit-helper                                                |
|                      | description: Generates conventional commit messages                    |
|                      | ---                                                                    |
|                      | # Git Commit Helper                                                    |
|                      | Instructions for formatting commit messages...                         |
+----------------------+------------------------------------------------------------------------+
| Canonical Store: ~/.agent-skills | 3 skills | Targets: 4                                      |
+-----------------------------------------------------------------------------------------------+
```

1. **Left Pane**: Search and filter skills in real time.
2. **Main (Top-Right)**: Interactive deployment matrix. Check/uncheck cells to enable/disable tools. Click **ℹ** for details and collision diagnostics.
3. **Editor (Bottom-Right)**: Edit `SKILL.md` with live frontmatter syntax and directory match validation.
4. **Toolbar Actions**:
   - **New Skill**: Modal form for name, description, and starting prompt.
   - **Import Existing**: Discovers real skills in target directories and moves them into canonical store.
   - **Sync All**: One-click synchronization for drifted copies and TOML files.
   - **Settings**: Add, remove, or modify target paths, modes, and formats.

---

## Safety & Collision Guarantees

1. **Never Overwrite**:
   If a destination path already exists and was not created by Skills Manager (not a symlink to canonical and not tracked in `state.json`), `Enable` refuses the operation with `StatusConflict`.
2. **Never Delete Foreign Files**:
   `Disable` only deletes symlinks pointing to canonical store or folders/files tracked in `state.json`. Unmanaged user directories are never touched.
3. **Atomic State Tracking**:
   State transitions are saved in `state.json` with timestamps, modes, and content hashes.

---

## Assumptions & Path Verification

The default targets in `targets.json` use standard default directory paths for each tool. Users should verify these against their installed CLI versions and customize them via the UI Settings dialog if their configuration differs:

| Tool | Default Path | Mode | Format | Verification Note |
| :--- | :--- | :--- | :--- | :--- |
| **Claude Code** | `~/.claude/skills` | `symlink` | `skill` | Standard location for user-defined Claude Code skills. |
| **Codex CLI** | `~/.codex/skills` | `symlink` | `skill` | Default skill directory for Codex CLI. |
| **OpenCode** | `~/.config/opencode/skill` | `symlink` | `skill` | Uses XDG config standard path on Linux/macOS. |
| **Gemini CLI** | `~/.gemini/commands` | `copy` | `gemini-toml` | Commands folder where `.toml` custom command prompts reside. |

*Note: Paths support `~` expansion, `$ENV` variables, and Windows `%VAR%` syntax. Target folders are automatically created upon first enable.*

---

## Testing & Quality Assurance

The codebase features comprehensive unit test coverage in `internal/core`:
- ✅ YAML Frontmatter parsing, validation, and serialization round-tripping.
- ✅ Symlink mode enable/disable and broken link detection.
- ✅ Copy mode enable/disable and recursive directory drift detection.
- ✅ Gemini CLI TOML conversion and synchronization.
- ✅ Collision protection and refusal of unmanaged files.
- ✅ Import of existing unmanaged skills and Gemini TOML commands.
- ✅ ZIP archive creation and nested file export.

Run unit tests:
```bash
go test -v -race ./internal/core/...
```
