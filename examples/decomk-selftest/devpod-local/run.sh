#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"

# Intent: Drive the full self-test through decomk itself: publish a temporary
# config repo over git://, let decomk pull it during postCreate, and validate
# pass/fail only from container logs.
# Source: DI-007-20260311-145221 (TODO/007)

usage() {
  cat <<'USAGE'
Usage:
  examples/decomk-selftest/devpod-local/run.sh [decomk run args...]

Examples:
  examples/decomk-selftest/devpod-local/run.sh
  examples/decomk-selftest/devpod-local/run.sh TUPLE_VERIFY_TOOL TUPLE_VERIFY_CONF TUPLE_CONTEXT_OVERRIDE TUPLE_DEFAULT_SHARED
  examples/decomk-selftest/devpod-local/run.sh all
USAGE
}

if [[ $# -eq 1 ]] && [[ "$1" == "-h" || "$1" == "--help" ]]; then
  usage
  exit 0
fi

decomk_args=("$@")
if [[ ${#decomk_args[@]} -eq 0 ]]; then
  decomk_args=(
    TUPLE_VERIFY_TOOL
    TUPLE_VERIFY_CONF
    TUPLE_CONTEXT_OVERRIDE
    TUPLE_DEFAULT_SHARED
  )
fi

for arg in "${decomk_args[@]}"; do
  # Intent: Context selection in this harness must be automatic from workspace
  # repo identity; do not allow explicit -context overrides in run.sh.
  # Source: DI-007-20260311-145221 (TODO/007)
  if [[ "$arg" == -* ]]; then
    die "run.sh accepts decomk action args only (flags such as $arg are not allowed)"
  fi
done

join_space_args() {
  local out=""
  local item
  for item in "$@"; do
    if [[ -n "$out" ]]; then
      out+=" "
    fi
    out+="$item"
  done
  printf '%s' "$out"
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[|&]/\\&/g'
}

need_command devpod
need_command docker
need_command git
ensure_docker_provider_selected

repo_root="$(cd "$script_dir/../../.." && pwd)"
template_dir="$script_dir/workspace-template/.devcontainer"
fixture_template="$repo_root/examples/decomk-selftest/fixtures/confrepo"
temp_root="$(mktemp -d /tmp/decomk-selftest.XXXXXX)"
active_workspaces=()

git_server_pid=""
git_server_log="$temp_root/git-daemon.log"
git_server_port=""
runtime_conf_repo="$temp_root/confrepo"
runtime_conf_bare="$temp_root/confrepo.git"
runtime_tool_bare="$temp_root/toolrepo.git"

cleanup() {
  local workspace_name
  for workspace_name in "${active_workspaces[@]:-}"; do
    cleanup_workspace "$workspace_name"
  done

  if [[ -n "${git_server_pid:-}" ]]; then
    kill "$git_server_pid" >/dev/null 2>&1 || true
    wait "$git_server_pid" >/dev/null 2>&1 || true
  fi

  if ! rm -rf "$temp_root"; then
    # Intent: Keep reruns ergonomic by attempting privileged cleanup when prior
    # container operations leave root-owned files in the temporary workspace.
    # Source: DI-007-20260309-124345 (TODO/007)
    sudo -n rm -rf "$temp_root" 2>/dev/null || true
  fi
}
trap cleanup EXIT

log "temporary workspace root: $temp_root"

prepare_fixture_repos() {
  # Intent: Keep test policy in fixture files, but publish it as a real git repo
  # so decomk stage-0 bootstrap exercises real tool+config clone/pull paths.
  # Source: DI-007-20260311-145221 (TODO/007)
  run_logged mkdir -p "$runtime_conf_repo"
  run_logged cp -a "$fixture_template/." "$runtime_conf_repo"
  run_logged git -C "$runtime_conf_repo" init -q
  run_logged git -C "$runtime_conf_repo" config user.name "decomk selftest"
  run_logged git -C "$runtime_conf_repo" config user.email "selftest@example.invalid"
  run_logged git -C "$runtime_conf_repo" add .
  run_logged git -C "$runtime_conf_repo" commit -q -m "Seed decomk selftest fixture repo"
  run_logged git clone -q --bare "$runtime_conf_repo" "$runtime_conf_bare"
  run_logged git clone -q --bare "$repo_root/.git" "$runtime_tool_bare"
}

start_git_server() {
  local attempt
  local port
  : >"$git_server_log"
  for attempt in $(seq 1 20); do
    port=$((RANDOM % 10000 + 20000))
    git daemon \
      --reuseaddr \
      --base-path="$temp_root" \
      --export-all \
      --listen=0.0.0.0 \
      --port="$port" \
      "$temp_root" \
      >"$git_server_log" 2>&1 &
    git_server_pid="$!"
    sleep 0.2
    if kill -0 "$git_server_pid" >/dev/null 2>&1; then
      git_server_port="$port"
      log "fixture git server ready on port $git_server_port"
      return 0
    fi
    wait "$git_server_pid" >/dev/null 2>&1 || true
    git_server_pid=""
  done
  die "failed to start fixture git server; see $git_server_log"
}

render_devcontainer_json() {
  local conf_repo_url="$1"
  local tool_repo_url="$2"
  local decomk_run_args="$3"
  local file_path="$4"
  local escaped_conf_url
  local escaped_tool_url
  local escaped_args

  escaped_conf_url="$(escape_sed_replacement "$conf_repo_url")"
  escaped_tool_url="$(escape_sed_replacement "$tool_repo_url")"
  escaped_args="$(escape_sed_replacement "$decomk_run_args")"
  sed -i \
    -e "s|__DECOMK_CONF_REPO__|$escaped_conf_url|g" \
    -e "s|__DECOMK_TOOL_REPO__|$escaped_tool_url|g" \
    -e "s|__DECOMK_RUN_ARGS__|$escaped_args|g" \
    "$file_path"
}

run_devpod_up() {
  local workspace_name="$1"
  local workspace_source="$2"

  # Intent: Keep one explicit source-first invocation shape so harness logs and
  # docs match exactly.
  # Source: DI-007-20260311-145221 (TODO/007)
  devpod up "$workspace_source" --id "$workspace_name" --ide none
}

latest_make_log_path() {
  devpod ssh "$workspace_name" --command "find /tmp/decomk-selftest/log -type f -name make.log | sort | tail -n1"
}

prepare_fixture_repos
start_git_server

workspace_copy="$temp_root/decomk"
workspace_name="dst-selftest-$(date +%s)-$$"
decomk_run_args="$(join_space_args "${decomk_args[@]}")"
conf_repo_url="git://host.docker.internal:$git_server_port/confrepo.git"
tool_repo_url="git://host.docker.internal:$git_server_port/toolrepo.git"
log "fixture config repo: $conf_repo_url"
log "fixture tool repo: $tool_repo_url"

log "preparing workspace"
run_logged mkdir -p "$workspace_copy"
run_logged cp -a "$repo_root/." "$workspace_copy"
run_logged rm -rf "$workspace_copy/.devcontainer"
run_logged mkdir -p "$workspace_copy/.devcontainer"
run_logged cp "$template_dir/devcontainer.json" "$workspace_copy/.devcontainer/devcontainer.json"
run_logged cp "$template_dir/Dockerfile" "$workspace_copy/.devcontainer/Dockerfile"
run_logged cp "$template_dir/postCreateCommand.sh" "$workspace_copy/.devcontainer/postCreateCommand.sh"
run_logged chmod +x "$workspace_copy/.devcontainer/postCreateCommand.sh"
render_devcontainer_json "$conf_repo_url" "$tool_repo_url" "$decomk_run_args" "$workspace_copy/.devcontainer/devcontainer.json"

active_workspaces+=("$workspace_name")
run_logged run_devpod_up "$workspace_name" "$workspace_copy"

make_log_path="$(latest_make_log_path)"
if [[ -z "$make_log_path" ]]; then
  die "self-test could not find make.log under /tmp/decomk-selftest/log"
fi
log "using make log: $make_log_path"

mapfile -t make_log_lines < <(devpod ssh "$workspace_name" --command "cat '$make_log_path'")

require_marker() {
  local marker="$1"
  local line
  for line in "${make_log_lines[@]}"; do
    if [[ "$line" == *"$marker"* ]]; then
      return 0
    fi
  done
  die "self-test marker not found in make.log: $marker"
}

require_absent_marker() {
  local marker="$1"
  local line
  for line in "${make_log_lines[@]}"; do
    if [[ "$line" == *"$marker"* ]]; then
      die "self-test marker unexpectedly present in make.log: $marker"
    fi
  done
}

require_no_fail_markers() {
  local line
  for line in "${make_log_lines[@]}"; do
    if [[ "$line" == *"SELFTEST FAIL"* ]]; then
      die "self-test failure marker found in make.log: $line"
    fi
  done
}

require_no_fail_markers

require_marker "SELFTEST PASS conf-repo-origin"
require_marker "SELFTEST PASS tool-repo-origin"
require_marker "SELFTEST PASS context-override"
require_marker "SELFTEST PASS default-tuple-available"

# Intent: Verify stamp semantics end-to-end by running one stamp target twice:
# first invocation executes and stamps; second invocation must skip that target,
# and the verifier target must confirm the probe ran exactly once.
# Source: DI-007-20260313-101500 (TODO/007)
run_logged devpod ssh "$workspace_name" --command "decomk run TUPLE_STAMP_PROBE"
stamp_probe_log_path="$(latest_make_log_path)"
if [[ -z "$stamp_probe_log_path" ]]; then
  die "self-test could not find stamp-probe make.log"
fi
mapfile -t make_log_lines < <(devpod ssh "$workspace_name" --command "cat '$stamp_probe_log_path'")
require_marker "SELFTEST PASS stamp-dir-working-dir"
require_marker "SELFTEST PASS stamp-probe-ran"

run_logged devpod ssh "$workspace_name" --command "decomk run TUPLE_STAMP_PROBE TUPLE_STAMP_VERIFY"
stamp_verify_log_path="$(latest_make_log_path)"
if [[ -z "$stamp_verify_log_path" ]]; then
  die "self-test could not find stamp-verify make.log"
fi
if [[ "$stamp_verify_log_path" == "$stamp_probe_log_path" ]]; then
  die "self-test stamp runs reused the same make.log path unexpectedly: $stamp_verify_log_path"
fi
mapfile -t make_log_lines < <(devpod ssh "$workspace_name" --command "cat '$stamp_verify_log_path'")
require_no_fail_markers
require_marker "SELFTEST PASS stamp-idempotent"
require_absent_marker "SELFTEST PASS stamp-probe-ran"

log "all required self-test markers found (including stamp checks)"
