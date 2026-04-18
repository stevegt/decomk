#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
source "$script_dir/lib.sh"

# Intent: Evaluate lifecycle assumptions empirically across devcontainer CLI,
# DevPod, and Codespaces so decomk stage-0 design decisions can rely on
# observed behavior instead of platform-documentation inference.
# Source: DI-009-20260418-130656 (TODO/009)

usage() {
  cat <<'USAGE'
Usage:
  examples/phase-eval/run.sh [options]

Options:
  --platform devcontainer|devpod|codespaces|all   Platform(s) to evaluate (default: all)
  --repo <owner/repo>                 GitHub repo slug for codespaces checks (default: inferred from origin)
  --branch <name>                     Branch for codespaces checks (default: current branch)
  --out-dir <path>                    Artifact directory (default: /tmp/decomk-phase-eval.<runid>)
  --keep-on-fail                      Keep remote workspaces/codespaces on failure
  --cleanup                           Remove local artifact directory on success
  -h, --help                          Show this help
USAGE
}

platform="all"
repo_slug=""
branch=""
out_dir=""
keep_on_fail="false"
cleanup_on_success="false"
codespaces_devcontainer_path=".devcontainer/phase-eval/devcontainer.json"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform)
      [[ $# -ge 2 ]] || die "--platform requires a value"
      platform="$2"
      shift 2
      ;;
    --repo)
      [[ $# -ge 2 ]] || die "--repo requires a value"
      repo_slug="$2"
      shift 2
      ;;
    --branch)
      [[ $# -ge 2 ]] || die "--branch requires a value"
      branch="$2"
      shift 2
      ;;
    --out-dir)
      [[ $# -ge 2 ]] || die "--out-dir requires a value"
      out_dir="$2"
      shift 2
      ;;
    --keep-on-fail)
      keep_on_fail="true"
      shift
      ;;
    --cleanup)
      cleanup_on_success="true"
      shift
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

case "$platform" in
  devcontainer|devpod|codespaces|all) ;;
  *) die "--platform must be one of: devcontainer, devpod, codespaces, all" ;;
esac

run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$$"
if [[ -z "$out_dir" ]]; then
  out_dir="/tmp/decomk-phase-eval.$run_id"
fi
mkdir -p "$out_dir/raw"

summary_path="$out_dir/summary.json"
: >"$out_dir/scenario-notes.tsv"

if [[ -z "$branch" ]]; then
  branch="$(git -C "$repo_root" rev-parse --abbrev-ref HEAD)"
fi

infer_repo_slug_from_origin() {
  local origin_url
  if ! origin_url="$(git -C "$repo_root" remote get-url origin 2>/dev/null)"; then
    return 1
  fi

  local owner=""
  local repo_name=""
  if [[ "$origin_url" =~ ^git@github\.com:([^/]+)/([^/]+)$ ]]; then
    owner="${BASH_REMATCH[1]}"
    repo_name="${BASH_REMATCH[2]}"
  elif [[ "$origin_url" =~ ^https://github\.com/([^/]+)/([^/]+)$ ]]; then
    owner="${BASH_REMATCH[1]}"
    repo_name="${BASH_REMATCH[2]}"
  else
    return 1
  fi

  repo_name="${repo_name%.git}"
  [[ -n "$owner" && -n "$repo_name" ]] || return 1
  printf '%s/%s\n' "$owner" "$repo_name"
}

if [[ -z "$repo_slug" ]]; then
  if repo_slug="$(infer_repo_slug_from_origin)"; then
    :
  else
    repo_slug=""
  fi
fi

json_string() {
  sanitize_json_string "$1"
}

bool_json() {
  local value="$1"
  if [[ "$value" == "true" ]]; then
    printf 'true'
  else
    printf 'false'
  fi
}

collect_events_from_logs() {
  local events_path="$1"
  shift

  local aggregate_path="$events_path.aggregate"
  : >"$aggregate_path"

  local log_path
  for log_path in "$@"; do
    if [[ -f "$log_path" ]]; then
      cat "$log_path" >>"$aggregate_path"
      printf '\n' >>"$aggregate_path"
    fi
  done

  # Intent: Extract hook events even when runtime loggers prefix each line
  # (for example Codespaces prebuild logs), by capturing the event token and
  # the remainder of the line instead of requiring a start-of-line match.
  # Source: DI-009-20260418-140800 (TODO/009)
  if rg -o 'PHASE_EVAL_EVENT\|.*' "$aggregate_path" >"$events_path"; then
    :
  else
    local rg_rc=$?
    if [[ "$rg_rc" -eq 1 ]]; then
      : >"$events_path"
    else
      rm -f "$aggregate_path"
      return "$rg_rc"
    fi
  fi

  rm -f "$aggregate_path"
}

event_has_hook() {
  local events_path="$1"
  local hook_name="$2"
  if [[ ! -f "$events_path" ]]; then
    return 1
  fi
  rg -q "\|hook=${hook_name}\|" "$events_path"
}

event_has_nonempty_field() {
  local events_path="$1"
  local field_name="$2"
  if [[ ! -f "$events_path" ]]; then
    return 1
  fi
  rg -q "\|${field_name}=[^|[:space:]][^|]*" "$events_path"
}

event_has_phase_hook() {
  local events_path="$1"
  local phase_bucket="$2"
  local hook_name="$3"
  if [[ ! -f "$events_path" ]]; then
    return 1
  fi
  rg -q "\|hook=${hook_name}\|.*phase_bucket=${phase_bucket}\|" "$events_path"
}

marker_list_has_marker() {
  local marker_list_path="$1"
  local marker_relpath="$2"
  if [[ ! -f "$marker_list_path" ]]; then
    return 1
  fi
  rg -q "^${marker_relpath}$" "$marker_list_path"
}

resolve_remote_branch_head_sha() {
  local repo="$1"
  local branch_name="$2"
  gh api "repos/$repo/branches/$branch_name" --jq '.commit.sha'
}

resolve_machine_name() {
  local requested_machine="$1"
  local repo="$2"
  local machine_lines=""

  if machine_lines="$(gh api "repos/$repo/codespaces/machines" --jq '.machines[].name' 2>/dev/null)"; then
    :
  elif machine_lines="$(gh api /user/codespaces/machines --jq '.machines[].name' 2>/dev/null)"; then
    :
  else
    return 1
  fi

  [[ -n "$machine_lines" ]] || return 1

  local available_names=()
  local entry
  while IFS= read -r entry; do
    if [[ -n "$entry" ]]; then
      available_names+=("$entry")
    fi
  done <<<"$machine_lines"

  [[ ${#available_names[@]} -gt 0 ]] || return 1

  if [[ -n "$requested_machine" ]]; then
    local candidate
    for candidate in "${available_names[@]}"; do
      if [[ "$candidate" == "$requested_machine" ]]; then
        printf '%s\n' "$candidate"
        return 0
      fi
    done
    return 2
  fi

  local preferred
  for preferred in basicLinux32gb standardLinux32gb basicLinux64gb standardLinux64gb; do
    for entry in "${available_names[@]}"; do
      if [[ "$entry" == "$preferred" ]]; then
        printf '%s\n' "$entry"
        return 0
      fi
    done
  done

  printf '%s\n' "${available_names[0]}"
  return 0
}

resolve_codespace_name() {
  local repo="$1"
  local display_name="$2"
  local timeout_seconds="$3"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    while IFS=$'\t' read -r listed_display listed_name; do
      if [[ "$listed_display" == "$display_name" && -n "$listed_name" ]]; then
        printf '%s\n' "$listed_name"
        return 0
      fi
    done < <(gh codespace list -R "$repo" --json displayName,name --jq '.[] | [.displayName, .name] | @tsv')
    sleep 5
  done

  return 1
}

remote_repo_file_exists() {
  local repo="$1"
  local branch_name="$2"
  local repo_path="$3"
  gh api --method GET "repos/$repo/contents/$repo_path" -f "ref=$branch_name" >/dev/null 2>&1
}

wait_for_codespace_ssh() {
  local codespace_name="$1"
  local timeout_seconds="$2"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    if gh codespace ssh -c "$codespace_name" -- "true" >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  return 1
}

resolve_prebuild_workflows() {
  local repo="$1"
  gh api "repos/$repo/actions/workflows" \
    --paginate \
    --jq '.workflows[] | select(.name | test("codespaces prebuild"; "i")) | [.id, .name, .path] | @tsv'
}

resolve_workflow_run_for_head() {
  local repo="$1"
  local workflow_id="$2"
  local branch_name="$3"
  local target_head_sha="$4"
  local timeout_seconds="$5"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    local run_rows=""
    if run_rows="$(gh run list -R "$repo" -w "$workflow_id" -b "$branch_name" --limit 50 --json databaseId,headSha,status,conclusion,createdAt --jq '.[] | [.databaseId, .headSha, .status, .conclusion, .createdAt] | @tsv' 2>/dev/null)"; then
      local run_id=""
      local head_sha=""
      local status=""
      local conclusion=""
      local created_at=""
      while IFS=$'\t' read -r run_id head_sha status conclusion created_at; do
        if [[ -z "$run_id" || -z "$head_sha" ]]; then
          continue
        fi
        if [[ "$head_sha" == "$target_head_sha" ]]; then
          printf '%s\t%s\t%s\t%s\n' "$run_id" "$status" "$conclusion" "$created_at"
          return 0
        fi
      done <<<"$run_rows"
    fi
    sleep 5
  done

  return 1
}

sanitize_devpod_workspace_id() {
  local raw="$1"
  local lowered
  lowered="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"
  lowered="$(printf '%s' "$lowered" | tr -c 'a-z0-9-' '-')"
  while [[ "$lowered" == *--* ]]; do
    lowered="${lowered//--/-}"
  done
  lowered="${lowered#-}"
  lowered="${lowered%-}"
  if [[ -z "$lowered" ]]; then
    lowered="phase-eval"
  fi
  printf '%s' "$lowered"
}

ensure_docker_provider_selected() {
  # Intent: Prefer read-only provider detection first so phase-eval can run in
  # environments where DevPod is already configured but config files are not
  # writable by the current process (for example restricted sandboxes).
  # Source: DI-009-20260415-081704 (TODO/009)
  if devpod provider list 2>/dev/null | rg -q 'docker'; then
    return 0
  fi

  if devpod provider use docker >/dev/null 2>&1; then
    return 0
  fi

  if devpod provider add docker >/dev/null 2>&1; then
    :
  fi
  # provider add is not idempotent; always retry selection and let the final
  # provider-use result determine success or failure.
  devpod provider use docker >/dev/null 2>&1
}

extract_container_id_from_devcontainer_stdout() {
  local stdout_path="$1"
  if [[ ! -f "$stdout_path" ]]; then
    return 1
  fi

  # Intent: Resolve the container id from any JSON event line that includes
  # `containerId` so parsing stays stable even when devcontainer emits extra
  # non-result lines before or after the final status line.
  # Source: DI-009-20260418-130656 (TODO/009)
  local container_id=""
  while IFS= read -r parsed_id; do
    if [[ -n "$parsed_id" ]]; then
      container_id="$parsed_id"
    fi
  done < <(jq -Rr 'fromjson? | if type == "object" then (.containerId // empty) else empty end' "$stdout_path" 2>/dev/null)
  if [[ -z "$container_id" ]]; then
    return 1
  fi
  printf '%s\n' "$container_id"
}

devcontainer_prebuild_rc=-1
devcontainer_prebuild_container_id=""
devcontainer_prebuild_on_create="false"
devcontainer_prebuild_update_content="false"
devcontainer_prebuild_post_create="false"
devcontainer_prebuild_github_user="false"
devcontainer_prebuild_user="false"
devcontainer_prebuild_cleanup_rc=-1

devcontainer_up_rc=-1
devcontainer_up_container_id=""
devcontainer_up_on_create="false"
devcontainer_up_update_content="false"
devcontainer_up_post_create="false"
devcontainer_up_github_user="false"
devcontainer_up_user="false"
devcontainer_up_cleanup_rc=-1

devpod_build_rc=-1
devpod_build_on_create="false"
devpod_build_update_content="false"
devpod_build_post_create="false"
devpod_build_github_user="false"
devpod_build_user="false"

devpod_up_rc=-1
devpod_up_on_create="false"
devpod_up_update_content="false"
devpod_up_post_create="false"
devpod_up_github_user="false"
devpod_up_user="false"

devpod_cleanup_rc=0

codespaces_auth_rc=-1
codespaces_push_check_rc=-1
codespaces_local_head_sha=""
codespaces_remote_head_sha=""
codespaces_head_match_remote="false"
codespaces_prebuild_list_rc=-1
codespaces_prebuild_trigger_rc=-1
codespaces_prebuild_watch_rc=-1
codespaces_prebuild_logs_rc=-1
codespaces_prebuild_head_sha=""
codespaces_prebuild_head_match_remote="false"
codespaces_prebuild_workflow_id=""
codespaces_prebuild_workflow_name=""
codespaces_prebuild_run_id=""
codespaces_prebuild_on_create="false"
codespaces_prebuild_update_content="false"
codespaces_prebuild_post_create="false"
codespaces_prebuild_github_user="false"
codespaces_prebuild_user="false"
codespaces_create_rc=-1
codespaces_create_on_create="false"
codespaces_update_content="false"
codespaces_post_create="false"
codespaces_github_user="false"
codespaces_user="false"
codespaces_logs_prebuild_hint="false"
codespaces_artifacts_fetch_rc=-1
codespaces_prebuild_marker_present="false"
codespaces_runtime_update_marker_present="false"
codespaces_runtime_postcreate_marker_present="false"
codespaces_persistent_prebuild_update_event="false"
codespaces_persistent_runtime_postcreate_event="false"
codespaces_cleanup_rc=0
codespace_name=""

if [[ "$platform" == "devcontainer" || "$platform" == "all" ]]; then
  need_command devcontainer
  need_command docker
  need_command git
  need_command jq

  # Intent: Evaluate devcontainer CLI lifecycle behavior directly by running one
  # prebuild-only `up --prebuild` scenario and one normal `up` scenario against
  # isolated workspace copies.
  # Source: DI-009-20260418-130656 (TODO/009)
  devcontainer_prebuild_workspace="$out_dir/workspace-devcontainer-prebuild"
  mkdir -p "$devcontainer_prebuild_workspace"
  cp -a "$repo_root/." "$devcontainer_prebuild_workspace"
  devcontainer_prebuild_config="$devcontainer_prebuild_workspace/.devcontainer/phase-eval/devcontainer.json"

  if run_capture "$out_dir" "devcontainer_prebuild" \
    devcontainer up \
    --workspace-folder "$devcontainer_prebuild_workspace" \
    --config "$devcontainer_prebuild_config" \
    --prebuild \
    --log-format json \
    --remote-env "PHASE_EVAL_RUN_ID=$run_id" \
    --remote-env "PHASE_EVAL_SCENARIO=devcontainer_prebuild"; then
    devcontainer_prebuild_rc=0
  else
    devcontainer_prebuild_rc=$?
  fi

  if devcontainer_prebuild_container_id="$(extract_container_id_from_devcontainer_stdout "$out_dir/raw/devcontainer_prebuild.stdout.log")"; then
    append_scenario_note "$out_dir" "devcontainer_prebuild" "container_id=$devcontainer_prebuild_container_id"
    if run_capture "$out_dir" "devcontainer_prebuild_fetch_events" docker cp "$devcontainer_prebuild_container_id:/tmp/decomk-phase-eval-hooks/events.log" "$out_dir/devcontainer-prebuild.events.log"; then
      :
    else
      append_scenario_note "$out_dir" "devcontainer_prebuild" "failed to fetch hook events via docker cp"
    fi
  else
    append_scenario_note "$out_dir" "devcontainer_prebuild" "failed to resolve container id from devcontainer output"
  fi

  if [[ -f "$out_dir/devcontainer-prebuild.events.log" ]]; then
    # Intent: Capture `onCreate` evidence for every evaluated platform/scenario
    # in phase-eval summary output while keeping this signal informational (no
    # new pass/fail gate) until lifecycle behavior is proven stable.
    # Source: DI-009-20260418-135200 (TODO/009)
    if event_has_hook "$out_dir/devcontainer-prebuild.events.log" "onCreate"; then
      devcontainer_prebuild_on_create="true"
    fi
    if event_has_hook "$out_dir/devcontainer-prebuild.events.log" "updateContent"; then
      devcontainer_prebuild_update_content="true"
    fi
    if event_has_hook "$out_dir/devcontainer-prebuild.events.log" "postCreate"; then
      devcontainer_prebuild_post_create="true"
    fi
    if event_has_nonempty_field "$out_dir/devcontainer-prebuild.events.log" "github_user"; then
      devcontainer_prebuild_github_user="true"
    fi
    if event_has_nonempty_field "$out_dir/devcontainer-prebuild.events.log" "user"; then
      devcontainer_prebuild_user="true"
    fi
  fi

  if [[ -n "$devcontainer_prebuild_container_id" ]]; then
    if [[ "$keep_on_fail" == "true" && "$devcontainer_prebuild_rc" -ne 0 ]]; then
      append_scenario_note "$out_dir" "devcontainer_prebuild" "container retained due to --keep-on-fail"
      devcontainer_prebuild_cleanup_rc=0
    else
      if run_capture "$out_dir" "devcontainer_prebuild_cleanup" docker rm -f "$devcontainer_prebuild_container_id"; then
        devcontainer_prebuild_cleanup_rc=0
      else
        devcontainer_prebuild_cleanup_rc=$?
        append_scenario_note "$out_dir" "devcontainer_prebuild" "container cleanup failed"
      fi
    fi
  else
    devcontainer_prebuild_cleanup_rc=0
  fi

  devcontainer_up_workspace="$out_dir/workspace-devcontainer-up"
  mkdir -p "$devcontainer_up_workspace"
  cp -a "$repo_root/." "$devcontainer_up_workspace"
  devcontainer_up_config="$devcontainer_up_workspace/.devcontainer/phase-eval/devcontainer.json"

  if run_capture "$out_dir" "devcontainer_up" \
    devcontainer up \
    --workspace-folder "$devcontainer_up_workspace" \
    --config "$devcontainer_up_config" \
    --log-format json \
    --remote-env "PHASE_EVAL_RUN_ID=$run_id" \
    --remote-env "PHASE_EVAL_SCENARIO=devcontainer_up"; then
    devcontainer_up_rc=0
  else
    devcontainer_up_rc=$?
  fi

  if devcontainer_up_container_id="$(extract_container_id_from_devcontainer_stdout "$out_dir/raw/devcontainer_up.stdout.log")"; then
    append_scenario_note "$out_dir" "devcontainer_up" "container_id=$devcontainer_up_container_id"
    if run_capture "$out_dir" "devcontainer_up_fetch_events" docker cp "$devcontainer_up_container_id:/tmp/decomk-phase-eval-hooks/events.log" "$out_dir/devcontainer-up.events.log"; then
      :
    else
      append_scenario_note "$out_dir" "devcontainer_up" "failed to fetch hook events via docker cp"
    fi
  else
    append_scenario_note "$out_dir" "devcontainer_up" "failed to resolve container id from devcontainer output"
  fi

  if [[ -f "$out_dir/devcontainer-up.events.log" ]]; then
    if event_has_hook "$out_dir/devcontainer-up.events.log" "onCreate"; then
      devcontainer_up_on_create="true"
    fi
    if event_has_hook "$out_dir/devcontainer-up.events.log" "updateContent"; then
      devcontainer_up_update_content="true"
    fi
    if event_has_hook "$out_dir/devcontainer-up.events.log" "postCreate"; then
      devcontainer_up_post_create="true"
    fi
    if event_has_nonempty_field "$out_dir/devcontainer-up.events.log" "github_user"; then
      devcontainer_up_github_user="true"
    fi
    if event_has_nonempty_field "$out_dir/devcontainer-up.events.log" "user"; then
      devcontainer_up_user="true"
    fi
  fi

  if [[ -n "$devcontainer_up_container_id" ]]; then
    if [[ "$keep_on_fail" == "true" && "$devcontainer_up_rc" -ne 0 ]]; then
      append_scenario_note "$out_dir" "devcontainer_up" "container retained due to --keep-on-fail"
      devcontainer_up_cleanup_rc=0
    else
      if run_capture "$out_dir" "devcontainer_up_cleanup" docker rm -f "$devcontainer_up_container_id"; then
        devcontainer_up_cleanup_rc=0
      else
        devcontainer_up_cleanup_rc=$?
        append_scenario_note "$out_dir" "devcontainer_up" "container cleanup failed"
      fi
    fi
  else
    devcontainer_up_cleanup_rc=0
  fi
fi

if [[ "$platform" == "devpod" || "$platform" == "all" ]]; then
  need_command devpod
  need_command docker
  need_command git

  if ensure_docker_provider_selected; then
    append_scenario_note "$out_dir" "devpod" "docker provider selected"
  else
    append_scenario_note "$out_dir" "devpod" "failed to select docker provider"
    die "unable to select DevPod docker provider"
  fi

  workspace_build="$out_dir/workspace-devpod-build"
  mkdir -p "$workspace_build"
  cp -a "$repo_root/." "$workspace_build"

  if run_capture "$out_dir" "devpod_build" devpod build "$workspace_build" --devcontainer-path .devcontainer/phase-eval/devcontainer.json --skip-push; then
    devpod_build_rc=0
  else
    devpod_build_rc=$?
  fi

  devpod_build_events="$out_dir/devpod-build.events.log"
  if collect_events_from_logs "$devpod_build_events" "$out_dir/raw/devpod_build.stdout.log" "$out_dir/raw/devpod_build.stderr.log"; then
    :
  else
    die "failed to extract devpod build events"
  fi

  if event_has_hook "$devpod_build_events" "updateContent"; then
    devpod_build_update_content="true"
  fi
  if event_has_hook "$devpod_build_events" "onCreate"; then
    devpod_build_on_create="true"
  fi
  if event_has_hook "$devpod_build_events" "postCreate"; then
    devpod_build_post_create="true"
  fi
  if event_has_nonempty_field "$devpod_build_events" "github_user"; then
    devpod_build_github_user="true"
  fi
  if event_has_nonempty_field "$devpod_build_events" "user"; then
    devpod_build_user="true"
  fi

  workspace_up="$out_dir/workspace-devpod-up"
  mkdir -p "$workspace_up"
  cp -a "$repo_root/." "$workspace_up"
  # Intent: Ensure generated DevPod workspace IDs always satisfy the provider
  # naming contract (lowercase letters, digits, dashes), even though run IDs
  # intentionally include uppercase UTC markers for artifact readability.
  # Source: DI-009-20260415-081608 (TODO/009)
  devpod_workspace_id="$(sanitize_devpod_workspace_id "phase-eval-${run_id}-up")"

  if run_capture "$out_dir" "devpod_up" \
    devpod up "$workspace_up" \
    --id "$devpod_workspace_id" \
    --ide none \
    --devcontainer-path .devcontainer/phase-eval/devcontainer.json \
    --workspace-env "PHASE_EVAL_RUN_ID=$run_id" \
    --workspace-env "PHASE_EVAL_SCENARIO=devpod_up"; then
    devpod_up_rc=0
  else
    devpod_up_rc=$?
  fi

  if [[ "$devpod_up_rc" -eq 0 ]]; then
    if run_capture "$out_dir" "devpod_up_fetch_events" devpod ssh "$devpod_workspace_id" --command "cat /tmp/decomk-phase-eval-hooks/events.log"; then
      :
    else
      append_scenario_note "$out_dir" "devpod_up" "failed to fetch hook events via ssh"
    fi

    if collect_events_from_logs "$out_dir/devpod-up.events.log" "$out_dir/raw/devpod_up.stdout.log" "$out_dir/raw/devpod_up.stderr.log" "$out_dir/raw/devpod_up_fetch_events.stdout.log"; then
      :
    else
      die "failed to extract devpod up events"
    fi

    if event_has_hook "$out_dir/devpod-up.events.log" "updateContent"; then
      devpod_up_update_content="true"
    fi
    if event_has_hook "$out_dir/devpod-up.events.log" "onCreate"; then
      devpod_up_on_create="true"
    fi
    if event_has_hook "$out_dir/devpod-up.events.log" "postCreate"; then
      devpod_up_post_create="true"
    fi
    if event_has_nonempty_field "$out_dir/devpod-up.events.log" "github_user"; then
      devpod_up_github_user="true"
    fi
    if event_has_nonempty_field "$out_dir/devpod-up.events.log" "user"; then
      devpod_up_user="true"
    fi
  fi

  if [[ "$keep_on_fail" == "true" && "$devpod_up_rc" -ne 0 ]]; then
    append_scenario_note "$out_dir" "devpod_up" "workspace retained due to --keep-on-fail"
  else
    if run_capture "$out_dir" "devpod_up_delete" devpod delete "$devpod_workspace_id"; then
      devpod_cleanup_rc=0
    else
      devpod_cleanup_rc=$?
      append_scenario_note "$out_dir" "devpod_up" "workspace cleanup failed"
    fi
  fi
fi

if [[ "$platform" == "codespaces" || "$platform" == "all" ]]; then
  need_command gh
  need_command git

  if [[ -z "$repo_slug" ]]; then
    append_scenario_note "$out_dir" "codespaces" "repo slug unavailable; codespaces scenarios not evaluated"
    codespaces_auth_rc=99
    codespaces_push_check_rc=99
    codespaces_prebuild_list_rc=99
    codespaces_prebuild_trigger_rc=99
    codespaces_prebuild_watch_rc=99
    codespaces_prebuild_logs_rc=99
    codespaces_create_rc=99
    codespaces_artifacts_fetch_rc=99
  else
    if run_capture "$out_dir" "codespaces_auth" gh auth status -t; then
      codespaces_auth_rc=0
    else
      codespaces_auth_rc=$?
    fi

    if [[ "$codespaces_auth_rc" -eq 0 ]]; then
      # Intent: Require local branch state to be pushed before prebuild/create
      # evaluation so all remote lifecycle checks run against the intended
      # commit and devcontainer path.
      # Source: DI-009-20260417-030759 (TODO/009)
      if run_capture "$out_dir" "codespaces_local_head" git -C "$repo_root" rev-parse HEAD; then
        codespaces_local_head_sha="$(cat "$out_dir/raw/codespaces_local_head.stdout.log")"
        codespaces_local_head_sha="${codespaces_local_head_sha//$'\n'/}"
      else
        codespaces_push_check_rc=$?
        append_scenario_note "$out_dir" "codespaces_push" "failed to resolve local HEAD"
      fi

      if [[ "$codespaces_push_check_rc" -eq -1 ]]; then
        if run_capture "$out_dir" "codespaces_remote_head" resolve_remote_branch_head_sha "$repo_slug" "$branch"; then
          codespaces_remote_head_sha="$(cat "$out_dir/raw/codespaces_remote_head.stdout.log")"
          codespaces_remote_head_sha="${codespaces_remote_head_sha//$'\n'/}"
        else
          codespaces_push_check_rc=$?
          append_scenario_note "$out_dir" "codespaces_push" "failed to resolve remote HEAD for $branch"
        fi
      fi

      if [[ "$codespaces_push_check_rc" -eq -1 ]]; then
        if [[ -n "$codespaces_local_head_sha" && -n "$codespaces_remote_head_sha" && "$codespaces_local_head_sha" == "$codespaces_remote_head_sha" ]]; then
          codespaces_push_check_rc=0
          codespaces_head_match_remote="true"
          append_scenario_note "$out_dir" "codespaces_push" "local HEAD matches origin/$branch ($codespaces_remote_head_sha)"
        else
          codespaces_push_check_rc=89
          append_scenario_note "$out_dir" "codespaces_push" "local HEAD does not match origin/$branch; commit+push required before evaluation"
        fi
      fi

      if [[ "$codespaces_push_check_rc" -eq 0 ]]; then
        if run_capture "$out_dir" "codespaces_prebuild_list" gh api "repos/$repo_slug/codespaces/prebuilds"; then
          codespaces_prebuild_list_rc=0
        else
          codespaces_prebuild_list_rc=$?
          append_scenario_note "$out_dir" "codespaces_prebuild" "prebuild list API unavailable; marking prebuild check inconclusive"
        fi
      else
        codespaces_prebuild_list_rc="$codespaces_push_check_rc"
        codespaces_prebuild_trigger_rc="$codespaces_push_check_rc"
        codespaces_prebuild_watch_rc="$codespaces_push_check_rc"
        codespaces_prebuild_logs_rc="$codespaces_push_check_rc"
        codespaces_create_rc="$codespaces_push_check_rc"
        codespaces_artifacts_fetch_rc="$codespaces_push_check_rc"
      fi

      if [[ "$codespaces_push_check_rc" -eq 0 ]]; then
      # Intent: Explicitly trigger and wait on a Codespaces prebuild workflow so
      # phase-eval can confirm hook execution during the prebuild lifecycle
      # instead of inferring behavior from docs or workspace startup side effects.
      # Source: DI-009-20260415-182322 (TODO/009)
      prebuild_workflows=""
      # Intent: Capture workflow discovery output in raw artifacts so missing
      # prebuild configuration is diagnosable without rerunning under debug.
      # Source: DI-009-20260415-182210 (TODO/009)
      if run_capture "$out_dir" "codespaces_prebuild_workflows" resolve_prebuild_workflows "$repo_slug"; then
        prebuild_workflows="$(cat "$out_dir/raw/codespaces_prebuild_workflows.stdout.log")"
        if [[ -n "$prebuild_workflows" ]]; then
          workflow_count="$(printf '%s\n' "$prebuild_workflows" | awk 'NF { count += 1 } END { print count + 0 }')"
          prebuild_workflow_row=""
          while IFS= read -r prebuild_workflow_row; do
            if [[ -n "$prebuild_workflow_row" ]]; then
              break
            fi
          done <<<"$prebuild_workflows"

          prebuild_workflow_path=""
          IFS=$'\t' read -r codespaces_prebuild_workflow_id codespaces_prebuild_workflow_name prebuild_workflow_path <<<"$prebuild_workflow_row"
          append_scenario_note "$out_dir" "codespaces_prebuild" "workflow=${codespaces_prebuild_workflow_name:-unknown} id=${codespaces_prebuild_workflow_id:-unknown} path=${prebuild_workflow_path:-unknown} matches=$workflow_count"

          if [[ -z "$codespaces_prebuild_workflow_id" ]]; then
            codespaces_prebuild_trigger_rc=92
            append_scenario_note "$out_dir" "codespaces_prebuild" "selected prebuild workflow row had no workflow id"
          else
            # Intent: Use the push-triggered Codespaces prebuild run for the
            # current remote HEAD instead of attempting workflow_dispatch, since
            # the built-in prebuild workflow is not dispatchable.
            # Source: DI-009-20260417-031847 (TODO/009)
            if run_capture "$out_dir" "codespaces_prebuild_find_run" resolve_workflow_run_for_head "$repo_slug" "$codespaces_prebuild_workflow_id" "$branch" "$codespaces_remote_head_sha" 1200; then
              prebuild_run_row="$(cat "$out_dir/raw/codespaces_prebuild_find_run.stdout.log")"
              prebuild_run_row="${prebuild_run_row//$'\n'/}"
              prebuild_status=""
              prebuild_conclusion=""
              prebuild_created_at=""
              IFS=$'\t' read -r codespaces_prebuild_run_id prebuild_status prebuild_conclusion prebuild_created_at <<<"$prebuild_run_row"
              codespaces_prebuild_head_sha="$codespaces_remote_head_sha"
              append_scenario_note "$out_dir" "codespaces_prebuild" "run_id=${codespaces_prebuild_run_id:-unknown} status=${prebuild_status:-unknown} conclusion=${prebuild_conclusion:-unknown} created_at=${prebuild_created_at:-unknown}"

              if run_capture "$out_dir" "codespaces_prebuild_watch" gh run watch "$codespaces_prebuild_run_id" -R "$repo_slug" --interval 15 --exit-status; then
                codespaces_prebuild_watch_rc=0
              else
                codespaces_prebuild_watch_rc=$?
              fi

              if run_capture "$out_dir" "codespaces_prebuild_logs" gh run view "$codespaces_prebuild_run_id" -R "$repo_slug" --log; then
                codespaces_prebuild_logs_rc=0
              else
                codespaces_prebuild_logs_rc=$?
              fi

              if collect_events_from_logs "$out_dir/codespaces-prebuild.events.log" "$out_dir/raw/codespaces_prebuild_logs.stdout.log" "$out_dir/raw/codespaces_prebuild_logs.stderr.log"; then
                :
              else
                die "failed to extract codespaces prebuild events"
              fi

              if event_has_hook "$out_dir/codespaces-prebuild.events.log" "updateContent"; then
                codespaces_prebuild_update_content="true"
              fi
              if event_has_hook "$out_dir/codespaces-prebuild.events.log" "onCreate"; then
                codespaces_prebuild_on_create="true"
              fi
              if event_has_hook "$out_dir/codespaces-prebuild.events.log" "postCreate"; then
                codespaces_prebuild_post_create="true"
              fi
              if event_has_nonempty_field "$out_dir/codespaces-prebuild.events.log" "github_user"; then
                codespaces_prebuild_github_user="true"
              fi
              if event_has_nonempty_field "$out_dir/codespaces-prebuild.events.log" "user"; then
                codespaces_prebuild_user="true"
              fi

              if [[ "$codespaces_prebuild_watch_rc" -eq 0 && "$codespaces_prebuild_logs_rc" -eq 0 ]]; then
                codespaces_prebuild_trigger_rc=0
              elif [[ "$codespaces_prebuild_watch_rc" -ne 0 ]]; then
                codespaces_prebuild_trigger_rc="$codespaces_prebuild_watch_rc"
              else
                codespaces_prebuild_trigger_rc="$codespaces_prebuild_logs_rc"
              fi

              if [[ -n "$codespaces_remote_head_sha" && -n "$codespaces_prebuild_head_sha" && "$codespaces_remote_head_sha" == "$codespaces_prebuild_head_sha" ]]; then
                codespaces_prebuild_head_match_remote="true"
              else
                append_scenario_note "$out_dir" "codespaces_prebuild" "prebuild run head sha does not match origin/$branch"
                if [[ "$codespaces_prebuild_trigger_rc" -eq 0 ]]; then
                  codespaces_prebuild_trigger_rc=90
                fi
              fi
            else
              codespaces_prebuild_trigger_rc=$?
              append_scenario_note "$out_dir" "codespaces_prebuild" "failed to resolve push-triggered prebuild workflow run for origin/$branch head"
            fi
          fi
        else
          codespaces_prebuild_trigger_rc=93
          append_scenario_note "$out_dir" "codespaces_prebuild" "no Codespaces prebuild workflow found; configure prebuilds first"
        fi
      else
        codespaces_prebuild_trigger_rc=$?
        append_scenario_note "$out_dir" "codespaces_prebuild" "failed to enumerate prebuild workflows"
      fi

      machine_name=""
      if machine_name="$(resolve_machine_name "" "$repo_slug")"; then
        append_scenario_note "$out_dir" "codespaces_create" "machine=$machine_name"
      else
        machine_rc=$?
        append_scenario_note "$out_dir" "codespaces_create" "failed to resolve machine type (rc=$machine_rc)"
        codespaces_create_rc=97
      fi

      # Intent: Fail early when the requested devcontainer path is not present
      # on the remote branch, because `gh codespace create` resolves paths from
      # GitHub repository contents, not local uncommitted files.
      # Source: DI-009-20260415-182210 (TODO/009)
      if [[ "$codespaces_create_rc" -eq -1 ]]; then
        if remote_repo_file_exists "$repo_slug" "$branch" "$codespaces_devcontainer_path"; then
          append_scenario_note "$out_dir" "codespaces_create" "remote devcontainer path confirmed: $codespaces_devcontainer_path"
        else
          codespaces_create_rc=91
          append_scenario_note "$out_dir" "codespaces_create" "remote devcontainer path missing on $branch: $codespaces_devcontainer_path (commit+push required)"
        fi
      fi

      if [[ "$codespaces_create_rc" -eq -1 ]]; then
        display_name="phase-eval-$run_id"
        if run_capture "$out_dir" "codespaces_create" gh codespace create -R "$repo_slug" -b "$branch" --machine "$machine_name" --devcontainer-path "$codespaces_devcontainer_path" -d "$display_name" --idle-timeout 30m --retention-period 1h --default-permissions; then
          codespaces_create_rc=0
        else
          codespaces_create_rc=$?
        fi

        if [[ "$codespaces_create_rc" -eq 0 ]]; then
          if codespace_name="$(resolve_codespace_name "$repo_slug" "$display_name" 900)"; then
            append_scenario_note "$out_dir" "codespaces_create" "codespace=$codespace_name"
          else
            append_scenario_note "$out_dir" "codespaces_create" "failed to resolve created codespace name"
            codespaces_create_rc=96
          fi
        fi

        if [[ "$codespaces_create_rc" -eq 0 ]]; then
          if wait_for_codespace_ssh "$codespace_name" 600; then
            :
          else
            append_scenario_note "$out_dir" "codespaces_create" "SSH readiness timeout"
            codespaces_create_rc=95
          fi
        fi

        if [[ "$codespaces_create_rc" -eq 0 ]]; then
          if run_capture "$out_dir" "codespaces_logs" gh codespace logs -c "$codespace_name"; then
            :
          else
            append_scenario_note "$out_dir" "codespaces_create" "failed to fetch codespace logs"
          fi

          if rg -qi 'prebuild' "$out_dir/raw/codespaces_logs.stdout.log"; then
            codespaces_logs_prebuild_hint="true"
          fi

          if run_capture "$out_dir" "codespaces_fetch_events" gh codespace ssh -c "$codespace_name" -- "bash -lc 'cat /tmp/decomk-phase-eval-hooks/events.log'"; then
            :
          else
            append_scenario_note "$out_dir" "codespaces_create" "failed to fetch event log via ssh"
          fi

          if collect_events_from_logs "$out_dir/codespaces.events.log" "$out_dir/raw/codespaces_logs.stdout.log" "$out_dir/raw/codespaces_logs.stderr.log" "$out_dir/raw/codespaces_fetch_events.stdout.log"; then
            :
          else
            die "failed to extract codespaces events"
          fi

          if event_has_hook "$out_dir/codespaces.events.log" "updateContent"; then
            codespaces_update_content="true"
          fi
          if event_has_hook "$out_dir/codespaces.events.log" "onCreate"; then
            codespaces_create_on_create="true"
          fi
          if event_has_hook "$out_dir/codespaces.events.log" "postCreate"; then
            codespaces_post_create="true"
          fi
          if event_has_nonempty_field "$out_dir/codespaces.events.log" "github_user"; then
            codespaces_github_user="true"
          fi
          if event_has_nonempty_field "$out_dir/codespaces.events.log" "user"; then
            codespaces_user="true"
          fi

          # Intent: Verify durable in-container artifacts written by hook probes
          # so we can differentiate prebuild execution from runtime execution
          # even after the prebuild log stream has finished.
          # Source: DI-009-20260417-030759 (TODO/009)
          if run_capture "$out_dir" "codespaces_fetch_persistent_events" gh codespace ssh -c "$codespace_name" -- "bash -lc 'cat \"\$HOME/.decomk-phase-eval-hooks/history/events.log\"'"; then
            :
          else
            codespaces_artifacts_fetch_rc=$?
            append_scenario_note "$out_dir" "codespaces_artifacts" "failed to fetch persistent hook events"
          fi

          if run_capture "$out_dir" "codespaces_fetch_persistent_markers" gh codespace ssh -c "$codespace_name" -- "bash -lc 'marker_root=\"\$HOME/.decomk-phase-eval-hooks/markers\"; if [[ -d \"\$marker_root\" ]]; then find \"\$marker_root\" -type f -print | sed \"s#^\$marker_root/##\" | sort; fi'"; then
            :
          else
            fetch_markers_rc=$?
            if [[ "$codespaces_artifacts_fetch_rc" -eq -1 ]]; then
              codespaces_artifacts_fetch_rc="$fetch_markers_rc"
            fi
            append_scenario_note "$out_dir" "codespaces_artifacts" "failed to fetch persistent marker paths"
          fi

          if collect_events_from_logs "$out_dir/codespaces-persistent.events.log" "$out_dir/raw/codespaces_fetch_persistent_events.stdout.log" "$out_dir/raw/codespaces_fetch_persistent_events.stderr.log"; then
            :
          else
            collect_persistent_rc=$?
            if [[ "$codespaces_artifacts_fetch_rc" -eq -1 ]]; then
              codespaces_artifacts_fetch_rc="$collect_persistent_rc"
            fi
            append_scenario_note "$out_dir" "codespaces_artifacts" "failed to parse persistent events"
          fi

          if [[ "$codespaces_artifacts_fetch_rc" -eq -1 ]]; then
            codespaces_artifacts_fetch_rc=0
          fi

          if event_has_phase_hook "$out_dir/codespaces-persistent.events.log" "prebuild" "updateContent"; then
            codespaces_persistent_prebuild_update_event="true"
          fi
          if event_has_phase_hook "$out_dir/codespaces-persistent.events.log" "runtime" "postCreate"; then
            codespaces_persistent_runtime_postcreate_event="true"
          fi

          if marker_list_has_marker "$out_dir/raw/codespaces_fetch_persistent_markers.stdout.log" "prebuild/updateContent.marker"; then
            codespaces_prebuild_marker_present="true"
          fi
          if marker_list_has_marker "$out_dir/raw/codespaces_fetch_persistent_markers.stdout.log" "runtime/updateContent.marker"; then
            codespaces_runtime_update_marker_present="true"
          fi
          if marker_list_has_marker "$out_dir/raw/codespaces_fetch_persistent_markers.stdout.log" "runtime/postCreate.marker"; then
            codespaces_runtime_postcreate_marker_present="true"
          fi
        fi

        if [[ -n "$codespace_name" ]]; then
          if [[ "$keep_on_fail" == "true" && "$codespaces_create_rc" -ne 0 ]]; then
            append_scenario_note "$out_dir" "codespaces_create" "codespace retained due to --keep-on-fail"
          else
            if run_capture "$out_dir" "codespaces_delete" gh codespace delete -c "$codespace_name" -f; then
              codespaces_cleanup_rc=0
            else
              codespaces_cleanup_rc=$?
              append_scenario_note "$out_dir" "codespaces_create" "codespace cleanup failed"
            fi
          fi
        fi
      fi
      fi
    else
      append_scenario_note "$out_dir" "codespaces" "gh auth unavailable; codespaces scenarios not evaluated"
      codespaces_push_check_rc=98
      codespaces_prebuild_list_rc=98
      codespaces_prebuild_trigger_rc=98
      codespaces_prebuild_watch_rc=98
      codespaces_prebuild_logs_rc=98
      codespaces_create_rc=98
      codespaces_artifacts_fetch_rc=98
    fi
  fi
fi

cat >"$summary_path" <<JSON
{
  "run_id": "$(json_string "$run_id")",
  "timestamp_utc": "$(json_string "$(date -u '+%Y-%m-%dT%H:%M:%SZ')")",
  "repo_root": "$(json_string "$repo_root")",
  "repo_slug": "$(json_string "$repo_slug")",
  "branch": "$(json_string "$branch")",
  "platform": "$(json_string "$platform")",
  "artifacts_dir": "$(json_string "$out_dir")",
  "scenarios": {
    "devcontainer_prebuild": {
      "rc": $devcontainer_prebuild_rc,
      "container_id": "$(json_string "$devcontainer_prebuild_container_id")",
      "onCreate_seen": $(bool_json "$devcontainer_prebuild_on_create"),
      "updateContent_seen": $(bool_json "$devcontainer_prebuild_update_content"),
      "postCreate_seen": $(bool_json "$devcontainer_prebuild_post_create"),
      "github_user_nonempty": $(bool_json "$devcontainer_prebuild_github_user"),
      "user_nonempty": $(bool_json "$devcontainer_prebuild_user"),
      "cleanup_rc": $devcontainer_prebuild_cleanup_rc
    },
    "devcontainer_up": {
      "rc": $devcontainer_up_rc,
      "container_id": "$(json_string "$devcontainer_up_container_id")",
      "onCreate_seen": $(bool_json "$devcontainer_up_on_create"),
      "updateContent_seen": $(bool_json "$devcontainer_up_update_content"),
      "postCreate_seen": $(bool_json "$devcontainer_up_post_create"),
      "github_user_nonempty": $(bool_json "$devcontainer_up_github_user"),
      "user_nonempty": $(bool_json "$devcontainer_up_user"),
      "cleanup_rc": $devcontainer_up_cleanup_rc
    },
    "devpod_build": {
      "rc": $devpod_build_rc,
      "onCreate_seen": $(bool_json "$devpod_build_on_create"),
      "updateContent_seen": $(bool_json "$devpod_build_update_content"),
      "postCreate_seen": $(bool_json "$devpod_build_post_create"),
      "github_user_nonempty": $(bool_json "$devpod_build_github_user"),
      "user_nonempty": $(bool_json "$devpod_build_user")
    },
    "devpod_up": {
      "rc": $devpod_up_rc,
      "onCreate_seen": $(bool_json "$devpod_up_on_create"),
      "updateContent_seen": $(bool_json "$devpod_up_update_content"),
      "postCreate_seen": $(bool_json "$devpod_up_post_create"),
      "github_user_nonempty": $(bool_json "$devpod_up_github_user"),
      "user_nonempty": $(bool_json "$devpod_up_user"),
      "cleanup_rc": $devpod_cleanup_rc
    },
    "codespaces_prebuild_list": {
      "rc": $codespaces_prebuild_list_rc
    },
    "codespaces_push": {
      "rc": $codespaces_push_check_rc,
      "local_head_sha": "$(json_string "$codespaces_local_head_sha")",
      "remote_head_sha": "$(json_string "$codespaces_remote_head_sha")",
      "head_match_remote": $(bool_json "$codespaces_head_match_remote")
    },
    "codespaces_prebuild_run": {
      "rc": $codespaces_prebuild_trigger_rc,
      "workflow_id": "$(json_string "$codespaces_prebuild_workflow_id")",
      "workflow_name": "$(json_string "$codespaces_prebuild_workflow_name")",
      "run_id": "$(json_string "$codespaces_prebuild_run_id")",
      "run_head_sha": "$(json_string "$codespaces_prebuild_head_sha")",
      "run_head_match_remote": $(bool_json "$codespaces_prebuild_head_match_remote"),
      "onCreate_seen": $(bool_json "$codespaces_prebuild_on_create"),
      "updateContent_seen": $(bool_json "$codespaces_prebuild_update_content"),
      "postCreate_seen": $(bool_json "$codespaces_prebuild_post_create"),
      "github_user_nonempty": $(bool_json "$codespaces_prebuild_github_user"),
      "user_nonempty": $(bool_json "$codespaces_prebuild_user"),
      "watch_rc": $codespaces_prebuild_watch_rc,
      "logs_rc": $codespaces_prebuild_logs_rc
    },
    "codespaces_create": {
      "rc": $codespaces_create_rc,
      "codespace_name": "$(json_string "$codespace_name")",
      "onCreate_seen": $(bool_json "$codespaces_create_on_create"),
      "updateContent_seen": $(bool_json "$codespaces_update_content"),
      "postCreate_seen": $(bool_json "$codespaces_post_create"),
      "github_user_nonempty": $(bool_json "$codespaces_github_user"),
      "user_nonempty": $(bool_json "$codespaces_user"),
      "logs_contain_prebuild_hint": $(bool_json "$codespaces_logs_prebuild_hint"),
      "artifacts_fetch_rc": $codespaces_artifacts_fetch_rc,
      "persistent_prebuild_update_marker_present": $(bool_json "$codespaces_prebuild_marker_present"),
      "persistent_runtime_update_marker_present": $(bool_json "$codespaces_runtime_update_marker_present"),
      "persistent_runtime_postcreate_marker_present": $(bool_json "$codespaces_runtime_postcreate_marker_present"),
      "persistent_prebuild_update_event_seen": $(bool_json "$codespaces_persistent_prebuild_update_event"),
      "persistent_runtime_postcreate_event_seen": $(bool_json "$codespaces_persistent_runtime_postcreate_event"),
      "cleanup_rc": $codespaces_cleanup_rc,
      "auth_rc": $codespaces_auth_rc
    }
  }
}
JSON

touch "$out_dir/diagnostics.complete"
log "summary written: $summary_path"

# Intent: Treat requested-platform gaps as failures so phase-eval cannot report
# success when lifecycle evidence is missing (for example auth/API skips or
# missing durable phase markers proving prebuild-vs-runtime hook execution).
# Source: DI-009-20260417-030759 (TODO/009)
overall_rc=0
if [[ "$platform" == "devcontainer" || "$platform" == "all" ]]; then
  # Intent: Keep devcontainer platform checks evidence-driven: prebuild must
  # show updateContent without postCreate, and runtime must show postCreate.
  # Source: DI-009-20260418-130656 (TODO/009)
  if [[ "$devcontainer_prebuild_rc" -ne 0 || "$devcontainer_up_rc" -ne 0 || "$devcontainer_prebuild_update_content" != "true" || "$devcontainer_prebuild_post_create" != "false" || "$devcontainer_up_post_create" != "true" ]]; then
    overall_rc=1
  fi
fi
if [[ "$platform" == "devpod" || "$platform" == "all" ]]; then
  if [[ "$devpod_build_rc" -ne 0 || "$devpod_up_rc" -ne 0 ]]; then
    overall_rc=1
  fi
fi
if [[ "$platform" == "codespaces" || "$platform" == "all" ]]; then
  if [[ "$codespaces_auth_rc" -ne 0 || "$codespaces_push_check_rc" -ne 0 || "$codespaces_prebuild_trigger_rc" -ne 0 || "$codespaces_create_rc" -ne 0 || "$codespaces_artifacts_fetch_rc" -ne 0 || "$codespaces_prebuild_marker_present" != "true" || "$codespaces_runtime_postcreate_marker_present" != "true" || "$codespaces_persistent_prebuild_update_event" != "true" || "$codespaces_persistent_runtime_postcreate_event" != "true" ]]; then
    overall_rc=1
  fi
fi

if [[ "$cleanup_on_success" == "true" ]]; then
  if [[ "$overall_rc" -eq 0 ]]; then
    rm -rf "$out_dir"
    log "cleanup complete: removed $out_dir"
  else
    log "--cleanup requested but at least one scenario failed; preserving $out_dir"
  fi
fi

if [[ "$overall_rc" -ne 0 ]]; then
  log "phase-eval incomplete or failed; inspect $summary_path and scenario-notes.tsv"
  exit 1
fi
