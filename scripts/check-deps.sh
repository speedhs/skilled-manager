#!/bin/sh
# Fail when the library imports the GUI, or the CLI imports a network client.
set -eu
cd "$(dirname "$0")/.."

core_deps=$(go list -deps ./pkg/core)
if printf '%s\n' "$core_deps" | grep -q 'fyne.io/fyne'; then
  echo "pkg/core imports Fyne" >&2
  exit 1
fi

cli_deps=$(go list -deps ./cmd/skills)
if printf '%s\n' "$cli_deps" | grep -E '^(net/http|net/smtp|golang.org/x/net)$'; then
  echo "cmd/skills imports a network package" >&2
  exit 1
fi
