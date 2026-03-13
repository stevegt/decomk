#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"

# Intent: Prove decomk pulled config from the harness git server by asserting
# the checked-out config repo exists at DECOMK_HOME/conf and has the expected
# origin URL.
# Source: DI-007-20260311-145221 (TODO/007)
selftest_require_env DECOMK_HOME
selftest_require_env DECOMK_CONF_REPO

conf_repo_dir="$DECOMK_HOME/conf"
selftest_require_file "$conf_repo_dir/decomk.conf"

if [[ ! -d "$conf_repo_dir/.git" ]]; then
  selftest_fail "config-repo-missing-git-dir"
fi

origin_url="$(git -C "$conf_repo_dir" config --get remote.origin.url || true)"
if [[ -z "$origin_url" ]]; then
  selftest_fail "config-repo-missing-origin"
fi
if [[ "$origin_url" != "$DECOMK_CONF_REPO" ]]; then
  selftest_fail "config-origin-mismatch expected=$DECOMK_CONF_REPO actual=$origin_url"
fi

echo "SELFTEST PASS conf-repo-origin"
