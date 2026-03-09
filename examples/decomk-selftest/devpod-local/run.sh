#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"

# Intent: Execute a deterministic local DevPod integration matrix that validates
# decomk bootstrap behavior under privileged, unprivileged, and sudo-unavailable
# conditions without mutating the repository's tracked files.
# Source: DI-007-20260309-124345 (TODO/007)

usage() {
  cat <<'USAGE'
Usage:
  examples/decomk-selftest/devpod-local/run.sh --scenario <name|all>

Scenarios:
  root_hook_owner_inferred
  non_root_default_make_as_root
  no_sudo_expect_fail
  no_sudo_make_as_user
  all
USAGE
}

selected_scenario="all"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --scenario)
      [[ $# -ge 2 ]] || die "--scenario requires a value"
      selected_scenario="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

case "$selected_scenario" in
  all)
    scenario_list=(
      root_hook_owner_inferred
      non_root_default_make_as_root
      no_sudo_expect_fail
      no_sudo_make_as_user
    )
    ;;
  root_hook_owner_inferred|non_root_default_make_as_root|no_sudo_expect_fail|no_sudo_make_as_user)
    scenario_list=("$selected_scenario")
    ;;
  *)
    die "invalid scenario: $selected_scenario"
    ;;
esac

need_command devpod
need_command docker
ensure_docker_provider_selected

repo_root="$(cd "$script_dir/../../.." && pwd)"
template_dir="$script_dir/workspace-template/.devcontainer"
temp_root="$(mktemp -d /tmp/decomk-selftest.XXXXXX)"
active_workspaces=()

cleanup() {
  local workspace_name
  for workspace_name in "${active_workspaces[@]:-}"; do
    cleanup_workspace "$workspace_name"
  done
  rm -rf "$temp_root"
}
trap cleanup EXIT

log "temporary workspace root: $temp_root"

run_devpod_up() {
  local workspace_name="$1"
  local workspace_source="$2"

  # Intent: Prefer the modern "name + --source" form, but keep a backward-
  # compatible fallback in case the local DevPod CLI expects "source + --id".
  # Source: DI-007-20260309-124345 (TODO/007)
  if devpod up "$workspace_name" --source "$workspace_source" --ide none; then
    return 0
  fi

  devpod up "$workspace_source" --id "$workspace_name" --ide none
}

for scenario_name in "${scenario_list[@]}"; do
  workspace_copy="$temp_root/workspace-$scenario_name"
  workspace_name="decomk-selftest-${scenario_name}-$(date +%s)-$$"

  log "preparing scenario: $scenario_name"
  run_logged mkdir -p "$workspace_copy"
  run_logged cp -a "$repo_root/." "$workspace_copy"
  run_logged rm -rf "$workspace_copy/.devcontainer"
  run_logged mkdir -p "$workspace_copy/.devcontainer"
  run_logged cp "$template_dir/devcontainer.json" "$workspace_copy/.devcontainer/devcontainer.json"
  run_logged cp "$template_dir/Dockerfile" "$workspace_copy/.devcontainer/Dockerfile"
  run_logged cp "$template_dir/postCreateCommand.sh" "$workspace_copy/.devcontainer/postCreateCommand.sh"
  run_logged chmod +x "$workspace_copy/.devcontainer/postCreateCommand.sh"
  printf '%s\n' "$scenario_name" > "$workspace_copy/.devcontainer/SELFTEST_SCENARIO"

  active_workspaces+=("$workspace_name")

  # Intent: Use explicit workspace id + source path so subsequent checks and
  # cleanup are deterministic and independent of local directory naming.
  # Source: DI-007-20260309-124345 (TODO/007)
  run_logged run_devpod_up "$workspace_name" "$workspace_copy"

  result_file="/tmp/decomk-selftest-results/$scenario_name/result.env"
  run_logged devpod ssh "$workspace_name" --command "test -f '$result_file'"

  outcome="$(devpod ssh "$workspace_name" --command "awk -F= '/^OUTCOME=/{print \$2}' '$result_file'")"
  summary="$(devpod ssh "$workspace_name" --command "awk -F= '/^SUMMARY=/{print substr(\$0,9)}' '$result_file'")"

  if [[ "$outcome" != "PASS" ]]; then
    die "scenario $scenario_name failed: ${summary:-no summary}"
  fi

  log "scenario $scenario_name PASS: ${summary:-ok}"

  cleanup_workspace "$workspace_name"
done

log "all requested scenarios passed"
