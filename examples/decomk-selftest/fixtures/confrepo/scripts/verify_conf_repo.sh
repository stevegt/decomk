#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"

# Intent: Prove decomk pulled config from the harness git server by asserting
# the checked-out config repo exists at DECOMK_HOME/conf and has the expected
# origin URL.
# Source: DI-007-20260412-171000 (TODO/007)
selftest_require_env DECOMK_HOME
selftest_require_env DECOMK_CONF_URI

conf_repo_dir="$DECOMK_HOME/conf"
selftest_require_file "$conf_repo_dir/decomk.conf"

if [[ ! -d "$conf_repo_dir/.git" ]]; then
  selftest_fail "config-repo-missing-git-dir"
fi

# Intent: Treat origin lookup failures as explicit selftest failures so
# repository-origin validation never silently passes.
# Source: DI-008-20260412-122157 (TODO/008)
if ! origin_url="$(git -C "$conf_repo_dir" config --get remote.origin.url 2>/dev/null)"; then
  selftest_fail "config-repo-missing-origin"
fi
if [[ -z "$origin_url" ]]; then
  selftest_fail "config-repo-missing-origin"
fi

expected_repo_url="$(selftest_git_uri_repo_url "$DECOMK_CONF_URI")"
if [[ "$origin_url" != "$expected_repo_url" ]]; then
  selftest_fail "config-origin-mismatch expected=$expected_repo_url actual=$origin_url"
fi

echo "SELFTEST PASS conf-repo-origin"
