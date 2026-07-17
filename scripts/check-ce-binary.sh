#!/usr/bin/env bash
# Fails if a community binary links any enterprise package. Uses a raw
# binary grep for the enterprise module path: the Go pclntab keeps package
# paths even in stripped (-s -w) binaries, where `go tool nm` finds nothing
# and would pass vacuously.
set -euo pipefail

binary="${1:?usage: check-ce-binary.sh <path-to-binary>}"

count=$(grep -a -c 'sqlwarden/enterprise' "$binary" || true)

if [ "$count" -gt 0 ]; then
  echo "FAIL: community binary references enterprise packages ($count matches)" >&2
  exit 1
fi
echo "OK: no enterprise package references in $binary"
