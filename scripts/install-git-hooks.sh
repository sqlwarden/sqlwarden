#!/usr/bin/env sh
set -eu

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

git config core.hooksPath scripts/git-hooks
chmod +x scripts/git-hooks/pre-commit

printf 'Configured git hooks path: %s\n' "$(git config core.hooksPath)"
