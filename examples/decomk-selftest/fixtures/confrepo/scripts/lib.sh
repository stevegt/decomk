#!/usr/bin/env bash

set -euo pipefail

# Intent: Keep reusable fixture assertions in one place so self-test targets stay
# small and emit consistent PASS/FAIL markers for harness log parsing.
# Source: DI-007-20260311-145221 (TODO/007)

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
