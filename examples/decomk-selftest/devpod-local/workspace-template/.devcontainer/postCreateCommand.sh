#!/usr/bin/env bash

set -euo pipefail

# Intent: Execute one deterministic decomk bootstrap scenario during container
# creation and persist a machine-readable result so the outer harness can verify
# behavior via DevPod SSH without parsing noisy setup logs.
# Source: DI-007-20260309-124345 (TODO/007)

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
workspace_root="$(cd "$script_dir/.." && pwd)"
scenario_file="$script_dir/SELFTEST_SCENARIO"
bootstrap_script="$workspace_root/examples/devcontainer/postCreateCommand.sh"

if [[ ! -f "$scenario_file" ]]; then
  echo "missing scenario file: $scenario_file" >&2
  exit 1
fi

if [[ ! -x "$bootstrap_script" ]]; then
  echo "missing bootstrap script: $bootstrap_script" >&2
  exit 1
fi

scenario_name="$(tr -d '[:space:]' < "$scenario_file")"
result_dir="/tmp/decomk-selftest-results/$scenario_name"
result_file="$result_dir/result.env"
state_root="/tmp/decomk-selftest/$scenario_name"

mkdir -p "$result_dir"
rm -rf "$state_root"

summary_message=""
failure_message=""

write_result() {
  local outcome="$1"
  local summary="$2"
  cat > "$result_file" <<RESULT
OUTCOME=$outcome
SUMMARY=$summary
RESULT
}

fail() {
  failure_message="$1"
  return 1
}

assert_file_exists() {
  local file_path="$1"
  [[ -f "$file_path" ]] || fail "expected file not found: $file_path"
}

assert_contains() {
  local file_path="$1"
  local expected_fragment="$2"
  grep -Fq -- "$expected_fragment" "$file_path" || fail "expected '$expected_fragment' in $file_path"
}

assert_nonempty_make_log() {
  local found_log
  found_log="$(find "$DECOMK_LOG_DIR" -type f -name make.log -print -quit 2>/dev/null || true)"
  [[ -n "$found_log" ]] || fail "no make.log found under $DECOMK_LOG_DIR"
  [[ -s "$found_log" ]] || fail "empty make.log at $found_log"
}

create_fixture_conf_repo() {
  local conf_repo_dir
  conf_repo_dir="/tmp/decomk-selftest-confrepo-$scenario_name"

  rm -rf "$conf_repo_dir"
  mkdir -p "$conf_repo_dir"

  cat > "$conf_repo_dir/decomk.conf" <<'CONF'
DEFAULT: Block00_base Block10_common
CONF

  cat > "$conf_repo_dir/Makefile" <<'MAKEFILE'
SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c
.RECIPEPREFIX := >

Block00_base:
>@echo "base"
>@touch $@

Block10_common: Block00_base
>@echo "common"
>@touch $@
MAKEFILE

  git -C "$conf_repo_dir" init -q
  git -C "$conf_repo_dir" config user.name "decomk selftest"
  git -C "$conf_repo_dir" config user.email "selftest@example.invalid"
  git -C "$conf_repo_dir" add decomk.conf Makefile
  git -C "$conf_repo_dir" commit -q -m "Add selftest fixture config"

  printf '%s\n' "$conf_repo_dir"
}

run_bootstrap_as_dev() {
  local fake_sudo_dir="$1"
  local make_as_root_value="$2"
  local runner_script
  local exit_code

  runner_script="$(mktemp /tmp/decomk-selftest-runner.XXXXXX.sh)"

  cat > "$runner_script" <<RUNNER
#!/usr/bin/env bash
set -euo pipefail
cd $(printf '%q' "$workspace_root")
export DECOMK_HOME=$(printf '%q' "$DECOMK_HOME")
export DECOMK_LOG_DIR=$(printf '%q' "$DECOMK_LOG_DIR")
export DECOMK_TOOL_REPO=$(printf '%q' "$DECOMK_TOOL_REPO")
export DECOMK_CONF_REPO=$(printf '%q' "$DECOMK_CONF_REPO")
if [[ -n $(printf '%q' "$fake_sudo_dir") ]]; then
  export PATH=$(printf '%q' "$fake_sudo_dir"):\$PATH
fi
if [[ -n $(printf '%q' "$make_as_root_value") ]]; then
  export DECOMK_MAKE_AS_ROOT=$(printf '%q' "$make_as_root_value")
fi
bash $(printf '%q' "$bootstrap_script")
RUNNER

  chmod +x "$runner_script"

  if /usr/bin/sudo -n -u dev -- /bin/bash "$runner_script"; then
    exit_code=0
  else
    exit_code=$?
  fi

  rm -f "$runner_script"
  return "$exit_code"
}

validate_success_common() {
  local expected_make_user="$1"
  local env_file
  local stamp_file

  env_file="$DECOMK_HOME/env.sh"
  stamp_file="$DECOMK_HOME/stamps/Block10_common"

  assert_file_exists "$env_file" || return 1
  assert_file_exists "$stamp_file" || return 1
  assert_nonempty_make_log || return 1
  assert_contains "$env_file" "DECOMK_MAKE_USER='$expected_make_user'" || return 1
  assert_contains "$env_file" "DECOMK_DEV_USER='dev'" || return 1

  summary_message="validated env/stamp/log invariants for scenario $scenario_name"
}

run_scenario_root_hook_owner_inferred() {
  (
    cd "$workspace_root"
    export DECOMK_HOME
    export DECOMK_LOG_DIR
    export DECOMK_TOOL_REPO
    export DECOMK_CONF_REPO
    export GITHUB_USER="dev"
    bash "$bootstrap_script"
  )

  validate_success_common "root"
}

run_scenario_non_root_default_make_as_root() {
  run_bootstrap_as_dev "" ""
  validate_success_common "root"
}

create_fake_sudo_dir() {
  local fake_sudo_dir
  fake_sudo_dir="$(mktemp -d /tmp/decomk-selftest-fakesudo.XXXXXX)"
  cat > "$fake_sudo_dir/sudo" <<'FAKESUDO'
#!/usr/bin/env bash
exit 1
FAKESUDO
  chmod +x "$fake_sudo_dir/sudo"
  printf '%s\n' "$fake_sudo_dir"
}

run_scenario_no_sudo_expect_fail() {
  local fake_sudo_dir
  local failure_log

  fake_sudo_dir="$(create_fake_sudo_dir)"
  failure_log="$result_dir/expected-failure.log"

  if run_bootstrap_as_dev "$fake_sudo_dir" "" >"$failure_log" 2>&1; then
    fail "expected bootstrap failure when sudo is unavailable"
    return 1
  fi

  if ! grep -Fq -- "need passwordless sudo" "$failure_log"; then
    fail "missing passwordless sudo error message"
    return 1
  fi

  summary_message="observed expected sudo failure when root-make is requested"
}

run_scenario_no_sudo_make_as_user() {
  local fake_sudo_dir

  fake_sudo_dir="$(create_fake_sudo_dir)"
  run_bootstrap_as_dev "$fake_sudo_dir" "false"
  validate_success_common "dev"
}

main() {
  export DECOMK_HOME="$state_root/home"
  export DECOMK_LOG_DIR="$state_root/log"
  export DECOMK_TOOL_REPO="$workspace_root"
  export DECOMK_CONF_REPO="$(create_fixture_conf_repo)"

  case "$scenario_name" in
    root_hook_owner_inferred)
      run_scenario_root_hook_owner_inferred
      ;;
    non_root_default_make_as_root)
      run_scenario_non_root_default_make_as_root
      ;;
    no_sudo_expect_fail)
      run_scenario_no_sudo_expect_fail
      ;;
    no_sudo_make_as_user)
      run_scenario_no_sudo_make_as_user
      ;;
    *)
      fail "unknown self-test scenario: $scenario_name"
      return 1
      ;;
  esac
}

if main; then
  write_result "PASS" "${summary_message:-scenario passed}"
  exit 0
fi

write_result "FAIL" "${failure_message:-scenario failed}"
exit 1
