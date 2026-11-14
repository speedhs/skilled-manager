# Contributing to Skills Manager

Thank you for contributing to Skills Manager.

## Development Setup

1. Prerequisites:
   - Go 1.25+
   - Make

2. Build and Test:
   ```bash
   # Run core test suite
   make test
   make lint
   make test-race

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

# 2. Create and push a v0 tag. v1 waits until the library API is frozen.
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

GitHub Actions will:
- Run the test suite.
- Build static CLI archives with GoReleaser, checksums, an SBOM, and a keyless cosign signature.
- Package unsigned desktop builds with Fyne. See `docs/RELEASE.md`.
