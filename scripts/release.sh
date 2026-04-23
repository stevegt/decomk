#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'USAGE' >&2
Usage:
  scripts/release.sh minor
USAGE
}

die() {
  echo "release.sh: $*" >&2
  exit 1
}

require_clean_repo() {
  local status
  status="$(git status --porcelain --untracked-files=normal)"
  if [[ -n "$status" ]]; then
    echo "$status" >&2
    die "repo has uncommitted or untracked changes"
  fi
}

parse_version() {
  local input="$1"
  if [[ "$input" =~ ^v?([0-9]+)\.([0-9]+)(\.([0-9]+))?$ ]]; then
    VERSION_MAJOR="${BASH_REMATCH[1]}"
    VERSION_MINOR="${BASH_REMATCH[2]}"
    return 0
  fi
  die "invalid VERSION value: $input (expected vMAJOR.MINOR[.PATCH])"
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
cd "$repo_root"

if [[ $# -ne 1 ]]; then
  usage
  exit 1
fi

release_kind="$1"
if [[ "$release_kind" != "minor" ]]; then
  usage
  die "unsupported release kind: $release_kind"
fi

# Intent: Keep release operations deterministic by refusing to run when the
# repository is dirty, then updating VERSION + generated version source as one
# atomic release commit before tagging and pushing.
# Source: DI-001-20260423-204251 (TODO/001)
require_clean_repo

if [[ ! -f VERSION ]]; then
  die "missing VERSION file at repo root"
fi

current_version="$(tr -d '[:space:]' < VERSION)"
if [[ -z "$current_version" ]]; then
  die "VERSION file is empty"
fi

parse_version "$current_version"
next_minor="$((VERSION_MINOR + 1))"
next_version="v${VERSION_MAJOR}.${next_minor}.0"

if git show-ref --verify --quiet "refs/tags/$next_version"; then
  die "tag already exists: $next_version"
fi

printf '%s\n' "$next_version" > VERSION
go run ./cmd/versiongen -repo-root .

git add VERSION cmd/decomk/version_generated.go
if git diff --cached --quiet; then
  die "release did not stage any changes"
fi

git commit -m "Release $next_version"
git tag "$next_version"
git push
git push --tags
