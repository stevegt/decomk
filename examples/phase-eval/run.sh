#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
source "$script_dir/lib.sh"

# Intent: Evaluate lifecycle assumptions empirically across DevPod and
# Codespaces so decomk stage-0 design decisions can rely on observed behavior
# instead of platform-documentation inference.
# Source: DI-009-20260413-232813 (TODO/009)

usage() {
  cat <<'USAGE'
Usage:
  examples/phase-eval/run.sh [options]

Options:
  --platform devpod|codespaces|both   Platform(s) to evaluate (default: both)
  --repo <owner/repo>                 GitHub repo slug for codespaces checks (default: inferred from origin)
  --branch <name>                     Branch for codespaces checks (default: current branch)
  --out-dir <path>                    Artifact directory (default: /tmp/decomk-phase-eval.<runid>)
  --keep-on-fail                      Keep remote workspaces/codespaces on failure
  --cleanup                           Remove local artifact directory on success
  -h, --help                          Show this help
USAGE
}

platform="both"
repo_slug=""
branch=""
out_dir=""
keep_on_fail="false"
cleanup_on_success="false"

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
  devpod|codespaces|both) ;;
  *) die "--platform must be one of: devpod, codespaces, both" ;;
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

  if rg '^PHASE_EVAL_EVENT\|' "$aggregate_path" >"$events_path"; then
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

ensure_docker_provider_selected() {
  if devpod provider use docker >/dev/null 2>&1; then
    return 0
  fi

  if devpod provider add docker >/dev/null 2>&1; then
    :
  else
    # provider add is not idempotent; selection retry decides outcome.
    :
  fi
  devpod provider use docker >/dev/null 2>&1
}

devpod_build_rc=-1
devpod_build_update_content="false"
devpod_build_post_create="false"
devpod_build_github_user="false"

devpod_up_rc=-1
devpod_up_update_content="false"
devpod_up_post_create="false"
devpod_up_github_user="false"

devpod_cleanup_rc=0

codespaces_auth_rc=-1
codespaces_prebuild_list_rc=-1
codespaces_create_rc=-1
codespaces_update_content="false"
codespaces_post_create="false"
codespaces_github_user="false"
codespaces_logs_prebuild_hint="false"
codespaces_cleanup_rc=0
codespace_name=""

if [[ "$platform" == "devpod" || "$platform" == "both" ]]; then
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
  if event_has_hook "$devpod_build_events" "postCreate"; then
    devpod_build_post_create="true"
  fi
  if event_has_nonempty_field "$devpod_build_events" "github_user"; then
    devpod_build_github_user="true"
  fi

  workspace_up="$out_dir/workspace-devpod-up"
  mkdir -p "$workspace_up"
  cp -a "$repo_root/." "$workspace_up"
  devpod_workspace_id="phase-eval-${run_id}-up"

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
    if event_has_hook "$out_dir/devpod-up.events.log" "postCreate"; then
      devpod_up_post_create="true"
    fi
    if event_has_nonempty_field "$out_dir/devpod-up.events.log" "github_user"; then
      devpod_up_github_user="true"
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

if [[ "$platform" == "codespaces" || "$platform" == "both" ]]; then
  need_command gh
  need_command git

  if [[ -z "$repo_slug" ]]; then
    append_scenario_note "$out_dir" "codespaces" "repo slug unavailable; skipping codespaces scenarios"
    codespaces_auth_rc=99
    codespaces_prebuild_list_rc=99
    codespaces_create_rc=99
  else
    if run_capture "$out_dir" "codespaces_auth" gh auth status -t; then
      codespaces_auth_rc=0
    else
      codespaces_auth_rc=$?
    fi

    if [[ "$codespaces_auth_rc" -eq 0 ]]; then
      if run_capture "$out_dir" "codespaces_prebuild_list" gh api "repos/$repo_slug/codespaces/prebuilds"; then
        codespaces_prebuild_list_rc=0
      else
        codespaces_prebuild_list_rc=$?
        append_scenario_note "$out_dir" "codespaces_prebuild" "prebuild list API unavailable; marking prebuild check inconclusive"
      fi

      machine_name=""
      if machine_name="$(resolve_machine_name "" "$repo_slug")"; then
        append_scenario_note "$out_dir" "codespaces_create" "machine=$machine_name"
      else
        machine_rc=$?
        append_scenario_note "$out_dir" "codespaces_create" "failed to resolve machine type (rc=$machine_rc)"
        codespaces_create_rc=97
      fi

      if [[ "$codespaces_create_rc" -ne 97 ]]; then
        display_name="phase-eval-$run_id"
        if run_capture "$out_dir" "codespaces_create" gh codespace create -R "$repo_slug" -b "$branch" --machine "$machine_name" --devcontainer-path .devcontainer/phase-eval/devcontainer.json -d "$display_name" --idle-timeout 30m --retention-period 1h --default-permissions; then
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
          if event_has_hook "$out_dir/codespaces.events.log" "postCreate"; then
            codespaces_post_create="true"
          fi
          if event_has_nonempty_field "$out_dir/codespaces.events.log" "github_user"; then
            codespaces_github_user="true"
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
    else
      append_scenario_note "$out_dir" "codespaces" "gh auth unavailable; skipping codespaces scenarios"
      codespaces_prebuild_list_rc=98
      codespaces_create_rc=98
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
    "devpod_build": {
      "rc": $devpod_build_rc,
      "updateContent_seen": $(bool_json "$devpod_build_update_content"),
      "postCreate_seen": $(bool_json "$devpod_build_post_create"),
      "github_user_nonempty": $(bool_json "$devpod_build_github_user")
    },
    "devpod_up": {
      "rc": $devpod_up_rc,
      "updateContent_seen": $(bool_json "$devpod_up_update_content"),
      "postCreate_seen": $(bool_json "$devpod_up_post_create"),
      "github_user_nonempty": $(bool_json "$devpod_up_github_user"),
      "cleanup_rc": $devpod_cleanup_rc
    },
    "codespaces_prebuild_list": {
      "rc": $codespaces_prebuild_list_rc
    },
    "codespaces_create": {
      "rc": $codespaces_create_rc,
      "codespace_name": "$(json_string "$codespace_name")",
      "updateContent_seen": $(bool_json "$codespaces_update_content"),
      "postCreate_seen": $(bool_json "$codespaces_post_create"),
      "github_user_nonempty": $(bool_json "$codespaces_github_user"),
      "logs_contain_prebuild_hint": $(bool_json "$codespaces_logs_prebuild_hint"),
      "cleanup_rc": $codespaces_cleanup_rc,
      "auth_rc": $codespaces_auth_rc
    }
  }
}
JSON

touch "$out_dir/diagnostics.complete"
log "summary written: $summary_path"

if [[ "$cleanup_on_success" == "true" ]]; then
  if [[ "$devpod_build_rc" -eq 0 && "$devpod_up_rc" -eq 0 && "$codespaces_create_rc" -eq 0 ]]; then
    rm -rf "$out_dir"
    log "cleanup complete: removed $out_dir"
  else
    log "--cleanup requested but at least one scenario failed; preserving $out_dir"
  fi
fi
