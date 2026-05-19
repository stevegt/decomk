#!/usr/bin/env bash

set -euo pipefail

# Intent: Keep reusable fixture assertions in one place so self-test targets stay
# small and emit consistent PASS/FAIL markers for harness log parsing.
# Source: DI-zulir (TODO-fuviv)

selftest_fail() {
  local message="$1"
  echo "SELFTEST FAIL $message"
  exit 1
}

selftest_require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    selftest_fail "missing-env-$name"
  fi
}

selftest_require_file() {
  local file_path="$1"
  if [[ ! -f "$file_path" ]]; then
    selftest_fail "missing-file-$file_path"
  fi
}

selftest_git_uri_repo_url() {
  local uri="$1"
  if [[ "$uri" != git:* ]]; then
    selftest_fail "invalid-git-uri-$uri"
  fi
  local payload="${uri#git:}"
  local repo_url="${payload%%\?*}"
  if [[ -z "$repo_url" ]]; then
    selftest_fail "invalid-git-uri-missing-url-$uri"
  fi
  printf '%s' "$repo_url"
}
