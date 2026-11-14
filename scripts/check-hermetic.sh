#!/bin/sh
# The behavioral check is TestHermeticGuardRefusesRealHome.
# This script only fails if that guard is no longer installed.
set -eu
cd "$(dirname "$0")/.."
for file in pkg/core/home_test.go internal/cli/cli_test.go; do
  if ! grep -q 'SKILLS_ORIGINAL_HOME' "$file"; then
    echo "$file does not capture SKILLS_ORIGINAL_HOME" >&2
    exit 1
  fi
done
