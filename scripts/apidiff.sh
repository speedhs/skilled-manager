#!/bin/sh
# v0 has no stable API. Once a v1 (or later) tag exists, compare pkg/core to it.
set -eu
cd "$(dirname "$0")/.."
# Only a release commit is gated. v1.0.0 is the pre-safety tag and is not a supported API.
tag=$(git describe --exact-match --tags HEAD 2>/dev/null || true)
case "$tag" in
""|v0.*|v1.0.0)
  echo "apidiff skipped (${tag:-untagged}); API remains v0"
  exit 0
  ;;
esac
echo "apidiff is required for ${tag}." >&2
echo "Install golang.org/x/exp/cmd/apidiff and compare pkg/core with ${tag} before this release." >&2
exit 1
