# Contributing to Skills Manager

Thank you for contributing to Skills Manager.

## Development Setup

1. Prerequisites:
   - Go 1.21+
   - Make

2. Build and Test:
   ```bash
   # Run core test suite
   make test

   # Build static CLI
   make build-cli

   # Run CLI
   go run ./cmd/skills list

   # Run GUI
   go run ./cmd/skills-gui
   ```

## Design Principles

- `pkg/core`: Pure Go, zero Fyne imports, completely covered by unit tests.
- `cmd/skills`: Pure static CLI compiled with `CGO_ENABLED=0`.
- `internal/ui`: Desktop interface calling only `pkg/core`.
- Safety: Never overwrite or delete untracked user files without explicit tracking or symlink confirmation.
- Style: Keep code, CLI outputs, and documentation clean and free of emojis.

## Creating a Release

Releases are automated via GitHub Actions on tag push:

```bash
# 1. Ensure all tests pass
make test

# 2. Create and push a semver tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

GitHub Actions will automatically:
- Run the full test suite.
- Build multi-platform static CLI archives via GoReleaser.
- Package desktop GUI applications via Fyne for Linux, macOS, and Windows.
- Publish artifacts to the GitHub Releases page with generated changelog notes.
