#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"

# Intent: Prove stage-0 bootstrap pulled decomk from the harness git server by
# asserting DECOMK_HOME/decomk exists as a git repo with the expected origin.
# Source: DI-007-20260311-145221 (TODO/007)
selftest_require_env DECOMK_HOME
selftest_require_env DECOMK_TOOL_REPO

tool_repo_dir="$DECOMK_HOME/decomk"
selftest_require_file "$tool_repo_dir/cmd/decomk/main.go"

if [[ ! -d "$tool_repo_dir/.git" ]]; then
  selftest_fail "tool-repo-missing-git-dir"
fi

origin_url="$(git -C "$tool_repo_dir" config --get remote.origin.url || true)"
if [[ -z "$origin_url" ]]; then
  selftest_fail "tool-repo-missing-origin"
fi
if [[ "$origin_url" != "$DECOMK_TOOL_REPO" ]]; then
  selftest_fail "tool-origin-mismatch expected=$DECOMK_TOOL_REPO actual=$origin_url"
fi

echo "SELFTEST PASS tool-repo-origin"
