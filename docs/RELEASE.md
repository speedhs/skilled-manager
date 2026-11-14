# Release

Releases from this tree are git tags `v0.x.y`. The module stays on `v0` until `pkg/core` is stable. The existing `v1.0.0` tag is the earlier proof of concept and is not a supported API. `scripts/apidiff.sh` does nothing on an untagged commit, on `v0`, and on that historical tag. A newer `v1` tag fails the script until an API diff is wired.

## What a tag builds

`/.github/workflows/release.yml` runs on a tag:

- GoReleaser builds static CLI binaries for linux, darwin, windows, and freebsd (`CGO_ENABLED=0`, `-trimpath`). Archives include checksums. The checksum file is signed with keyless cosign (`id-token: write`) when the workflow has an OIDC token. Syft writes an SBOM for the archives when `syft` is on `PATH` (the workflow installs it).
- A pull request runs `goreleaser release --snapshot --clean --skip=publish,sign,sbom` and does not publish.
- Fyne packages the GUI on Linux, macOS, and Windows. Those artifacts are **unsigned**. `packaging/UNSIGNED.txt` is the first-run note and is copied next to the GUI archive.

## First run of an unsigned GUI

- macOS Gatekeeper: Control-click the app, choose Open. This project has no Developer ID and no notarization ticket.
- Windows SmartScreen: "More info", then "Run anyway". There is no Authenticode signature.
- Linux: prefer the checksum file over the filename. The `.desktop` entry is `packaging/linux/skills-manager.desktop`. The icon is `packaging/skills-manager.png`.

Homebrew and Scoop manifests are generated with upload skipped until `speedhs/homebrew-tap` and `speedhs/scoop-bucket` exist. `.deb` and `.rpm` are built with nfpm as part of GoReleaser. `go install github.com/speedhs/skilled-manager/cmd/skills@latest` works only after the matching commit is on the default branch of that module path.

## Version

```
go build -trimpath -ldflags "-X github.com/speedhs/skilled-manager/internal/version.Version=v0.0.0 -X github.com/speedhs/skilled-manager/internal/version.Commit=abc -X github.com/speedhs/skilled-manager/internal/version.Date=2026-11-14T00:00:00Z" -o skills ./cmd/skills
```

`skills version` and the GUI About dialog print those three values. A local `make build-cli` fills them from git.

## Local snapshot

```
make release-snapshot
```

That requires GoReleaser 2. It skips publish, sign, and SBOM so it can run without cosign or syft.
