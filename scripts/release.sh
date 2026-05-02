#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'USAGE' >&2
Usage:
  scripts/release.sh minor
  scripts/release.sh promote-testing
  scripts/release.sh promote-stable
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

current_branch() {
  local branch
  if ! branch="$(git symbolic-ref --quiet --short HEAD)"; then
    die "current HEAD is detached; checkout a branch first"
  fi
  printf '%s\n' "$branch"
}

require_current_branch() {
  local expected="$1"
  local current
  current="$(current_branch)"
  if [[ "$current" != "$expected" ]]; then
    die "current branch must be $expected (got $current)"
  fi
}

remote_branch_exists() {
  local branch="$1"
  git show-ref --verify --quiet "refs/remotes/origin/$branch"
}

require_remote_ancestor() {
  local remote_branch="$1"
  local source_ref="$2"
  local purpose="$3"
  if remote_branch_exists "$remote_branch"; then
    if ! git merge-base --is-ancestor "refs/remotes/origin/$remote_branch" "$source_ref"; then
      die "$purpose would not be a fast-forward from origin/$remote_branch"
    fi
  fi
}

promote_branch_channel() {
  local source_branch="$1"
  local dest_branch="$2"

  # Intent: Keep moving branch channels (`testing`, `stable`) explicit and
  # fast-forward-only so stage-0/go-install consumers can follow predictable
  # repo-controlled release channels.
  # Source: DI-001-20260502-233406 (TODO/001)
  require_clean_repo
  git fetch origin
  require_current_branch "$source_branch"
  require_remote_ancestor "$source_branch" "HEAD" "source branch push"
  require_remote_ancestor "$dest_branch" "HEAD" "channel promotion"
  git push origin "HEAD:refs/heads/$source_branch"
  git push origin "HEAD:refs/heads/$dest_branch"
  echo "Promoted $source_branch -> $dest_branch at $(git rev-parse --short HEAD)"
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
case "$release_kind" in
  minor)
    # Intent: Keep immutable release operations deterministic by refusing to run
    # when the repository is dirty, then updating VERSION + generated version
    # source as one atomic release commit before tagging and pushing.
    # Source: DI-001-20260423-204251 (TODO/001)
    require_clean_repo
    require_current_branch "main"

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
    git push origin "HEAD:refs/heads/main"
    git push origin --tags
    ;;
  promote-testing)
    promote_branch_channel "main" "testing"
    ;;
  promote-stable)
    promote_branch_channel "testing" "stable"
    ;;
  *)
    usage
    die "unsupported release kind: $release_kind"
    ;;
esac
