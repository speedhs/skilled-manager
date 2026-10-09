# Skills Manager 🛠️

**Skills Manager** is a cross-platform native desktop application and CLI written in Go using [Fyne](https://fyne.io/) (v2.5+) that organizes and manages AI agent "skills" (`SKILL.md` folders) from a single canonical store and exposes them to multiple AI tools: **Claude Code**, **Codex CLI**, **OpenCode**, and **Gemini CLI**.

The core engine is located in [`pkg/core`](./pkg/core) as a clean, standalone, pure Go library with zero GUI dependencies, ready to be imported into any Go application or agent runtime.

[![Go Reference](https://pkg.go.dev/badge/github.com/skilled-manager/skills-manager/pkg/core.svg)](https://pkg.go.dev/github.com/skilled-manager/skills-manager/pkg/core)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## 📑 Table of Contents
- [Installation](#installation)
  - [Install CLI via Go](#install-cli-via-go)
  - [Download Pre-Built Binaries](#download-pre-built-binaries)
  - [Install Desktop GUI](#install-desktop-gui)
  - [Build from Source](#build-from-source)
- [Using as a Go Library (`pkg/core`)](#using-as-a-go-library-pkgcore)
- [Architecture Overview](#architecture-overview)
- [Core Model & Storage](#core-model--storage)
- [Tool Target Matrix](#tool-target-matrix)
- [Features](#features)
- [CLI Usage](#cli-usage)
- [GUI Walkthrough](#gui-walkthrough)
- [Safety & Collision Guarantees](#safety--collision-guarantees)
- [Assumptions & Path Verification](#assumptions--path-verification)
- [Testing & Quality Assurance](#testing--quality-assurance)

---

## Installation

### Install CLI via Go

You can install the `skills` CLI directly using `go install`. The CLI builds completely statically without CGO (`CGO_ENABLED=0`):

```bash
CGO_ENABLED=0 go install github.com/skilled-manager/skills-manager/cmd/skills@latest
```

Verify the installation:
```bash
skills list
```

### Download Pre-Built Binaries

Pre-compiled static binaries for macOS, Linux, Windows, and FreeBSD are published with every release:

1. Visit the [Releases](https://github.com/skilled-manager/skills-manager/releases) page.
2. Download the archive for your operating system and architecture:
   - Linux: `skills-manager-cli_<version>_linux_amd64.tar.gz` (or `arm64`)
   - macOS: `skills-manager-cli_<version>_darwin_all.tar.gz`
   - Windows: `skills-manager-cli_<version>_windows_amd64.zip`
3. Extract the binary and place it in your `$PATH` (e.g. `/usr/local/bin` or `C:\Windows\System32`).

### Install Desktop GUI

Native packaged desktop releases are built via `fyne package` for each platform:
- **macOS**: Download `skills-gui-darwin-universal.zip`, extract `Skills Manager.app`, and drag it into `/Applications`.
- **Windows**: Download `skills-gui-windows-amd64.zip`, extract, and run `skills-gui.exe`.
- **Linux**: Download `skills-gui-linux-amd64.tar.gz`, extract, and launch the binary.

### Build from Source

#### Prerequisites
- **Go 1.21+**
- On Linux (only needed for compiling the GUI binary, not the CLI):
  ```bash
  sudo apt-get install -y libgl1-mesa-dev xorg-dev libwayland-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev
  ```

#### Building
```bash
# Clone the repository
git clone https://github.com/skilled-manager/skills-manager.git
cd skills-manager

# Build pure static CLI binary
make build-cli

# Run Desktop GUI directly
make run-gui

# Run unit tests
make test
```

---

## Using as a Go Library (`pkg/core`)

The `pkg/core` package provides a standalone Go API for managing agent skills, evaluating deployment states, computing directory drift, and converting skills between formats without any GUI or CGO requirements.

### Installation
```bash
go get github.com/skilled-manager/skills-manager/pkg/core
```

### Code Example

```go
package main

import (
	"fmt"
	"log"

	"github.com/skilled-manager/skills-manager/pkg/core"
)

func main() {
	// Initialize manager with custom or default paths
	mgr, err := core.NewManager(
		core.WithCanonicalDir("~/.agent-skills"),
		core.WithConfigDir("~/.agent-skills-manager"),
	)
	if err != nil {
		log.Fatalf("Failed to initialize manager: %v", err)
	}

	// 1. Create a new skill
	skill, err := core.CreateSkill(
		mgr.CanonicalDir,
		"code-reviewer",
		"Reviews pull requests for performance and security",
		"# Code Reviewer\n\nEnsure all errors are handled and memory leaks checked.",
	)
	if err != nil {
		log.Printf("Skill might already exist: %v", err)
	} else {
		fmt.Printf("Created skill: %s\n", skill.Name)
	}

	// 2. List all skills
	skills, err := core.ListSkills(mgr.CanonicalDir)
	if err != nil {
		log.Fatalf("Failed to list skills: %v", err)
	}

	// 3. Enable for a specific target
	res, err := mgr.Enable("code-reviewer", "claude")
	if err != nil {
		log.Printf("Enable error: %v", err)
	} else {
		fmt.Printf("Enabled on Claude: %s (%s)\n", res.TargetPath, res.Mode)
	}

	// 4. Inspect status
	status, _ := mgr.Status("code-reviewer", "claude")
	fmt.Printf("Status on Claude: %s (%s)\n", status.Code, status.Message)

	// 5. Synchronize all drifted copies & TOML files
	report, err := mgr.Sync()
	if err != nil {
		log.Fatalf("Sync error: %v", err)
	}
	fmt.Printf("Sync report: %d updated, %d skipped\n", len(report.Updated), len(report.Skipped))
}
```

---

## Architecture Overview

```
skills-manager/
├── .github/
│   └── workflows/
│       └── release.yml          # Automated CI/CD release workflow (GoReleaser + Fyne packaging)
├── .goreleaser.yaml             # Multi-platform static CLI build configuration (CGO off)
├── cmd/
│   ├── skills/                  # Command-line interface
│   │   └── main.go              # CLI entrypoint (CGO_ENABLED=0)
│   └── skills-gui/              # Native Fyne GUI application
│       └── main.go              # Desktop GUI entrypoint
├── pkg/
│   └── core/                    # Pure Go library (Zero Fyne imports, importable)
│       ├── config.go            # Target configurations & defaults
│       ├── enable.go            # Enable, Disable, and Sync operations
│       ├── export.go            # ZIP export of canonical skills & nested folders
│       ├── frontmatter.go       # YAML frontmatter parsing, validation & serialization
│       ├── gemini.go            # Gemini CLI TOML conversion (<skill>.toml)
│       ├── hash.go              # Deterministic directory & file hashing for drift detection
│       ├── import.go            # Scanning & importing existing target skills
│       ├── link.go              # Cross-platform symlink/junction/copy fallback
│       ├── manager.go           # Central Manager coordinator
│       ├── path.go              # Path expansion (~, $ENV, %VAR%)
│       ├── skill.go             # Canonical skill CRUD & parsing
│       ├── state.go             # State tracking (~/.agent-skills-manager/state.json)
│       ├── status.go            # Cell status evaluation (enabled/disabled/conflict/drifted/broken)
│       ├── watcher.go           # fsnotify watcher with debounced event dispatch
│       ├── core_test.go         # Core unit test suite
│       └── gemini_test.go       # Gemini TOML unit test suite
├── internal/
│   └── ui/                      # Fyne desktop UI layer (calls only pkg/core)
│       ├── app.go               # Main window layout, split views, watcher integration
│       ├── dialogs.go           # New Skill, Import Existing, Settings, Sync Report dialogs
│       ├── editor.go            # Multi-line SKILL.md editor with frontmatter validation
│       ├── matrix.go            # Skills x Tools interactive matrix
│       ├── skill_list.go        # Searchable skill list pane
│       ├── toolbar.go           # Top action toolbar
│       ├── types.go             # UI constants, status color palette
│       └── ui_test.go           # UI component test suite
├── Makefile                     # Build & test automation
├── LICENSE                      # MIT License
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
- **Pure Go CLI with Zero CGO**: Compile and run anywhere without C dependencies (`CGO_ENABLED=0`).
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

## CLI Usage

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

The codebase features comprehensive unit test coverage in `pkg/core`:
- ✅ YAML Frontmatter parsing, validation, and serialization round-tripping.
- ✅ Symlink mode enable/disable and broken link detection.
- ✅ Copy mode enable/disable and recursive directory drift detection.
- ✅ Gemini CLI TOML conversion and synchronization.
- ✅ Collision protection and refusal of unmanaged files.
- ✅ Import of existing unmanaged skills and Gemini TOML commands.
- ✅ ZIP archive creation and nested file export.

Run unit tests:
```bash
make test
```

---

## License

This project is licensed under the [MIT License](./LICENSE).
