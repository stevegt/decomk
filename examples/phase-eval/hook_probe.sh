#!/usr/bin/env bash

set -euo pipefail

hook_name="${1:-}"
if [[ -z "$hook_name" ]]; then
  echo "phase-eval hook probe: missing hook name argument" >&2
  exit 2
fi

if [[ -n "${PHASE_EVAL_RUN_ID:-}" ]]; then
  run_id="${PHASE_EVAL_RUN_ID}"
elif [[ -n "${CODESPACE_NAME:-}" ]]; then
  run_id="${CODESPACE_NAME}"
else
  run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$$"
fi
scenario="${PHASE_EVAL_SCENARIO:-unknown}"

# Intent: Persist hook observations to both volatile (`/tmp`) and durable
# (`$HOME`) locations so phase-eval can prove whether hooks executed during
# prebuild (baked into snapshot) or during first boot/runtime.
# Source: DI-009-20260417-030759 (TODO/009)
volatile_root="/tmp/decomk-phase-eval-hooks"
persistent_root="${HOME:-/home/dev}/.decomk-phase-eval-hooks"
persistent_history_path="$persistent_root/history/events.log"
prebuild_update_marker_path="$persistent_root/markers/prebuild/updateContent.marker"

# Intent: Infer prebuild-vs-runtime phase for Codespaces without relying on
# undocumented environment variables by using durable marker/history state.
# Source: DI-009-20260417-030759 (TODO/009)
resolve_phase_bucket() {
  local hook="$1"
  local history_exists="$2"
  local prebuild_marker_seen="$3"
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    printf 'prebuild'
    return
  fi
  if [[ -n "${CODESPACES:-}" ]]; then
    # Intent: Keep Codespaces prebuild classification stable now that
    # `onCreateCommand` can run before `updateContentCommand`; treat onCreate
    # and updateContent as prebuild until the prebuild update marker exists.
    # Source: DI-009-20260418-140800 (TODO/009)
    if [[ "$prebuild_marker_seen" == "true" ]]; then
      printf 'runtime'
      return
    fi
    if [[ "$hook" == "postCreate" ]]; then
      printf 'runtime'
      return
    fi
    if [[ "$history_exists" == "true" && "$hook" == "onCreate" ]]; then
      printf 'runtime'
      return
    fi
    printf 'prebuild'
    return
  fi
  printf 'local'
}

resolve_phase_detail() {
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    printf 'github-actions'
    return
  fi
  if [[ -n "${CODESPACES:-}" ]]; then
    printf 'codespaces'
    return
  fi
  if [[ -n "${DEVPOD_WORKSPACE_ID:-}" ]]; then
    printf 'devpod'
    return
  fi
  printf 'local'
}

mkdir -p \
  "$volatile_root/env" \
  "$volatile_root/markers" \
  "$persistent_root/history" \
  "$persistent_root/env" \
  "$persistent_root/markers"

timestamp="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
volatile_env_path="$volatile_root/env/${hook_name}-${run_id}.env"
volatile_marker_path="$volatile_root/markers/${hook_name}-${run_id}.marker"
persistent_env_path="$persistent_root/env/${hook_name}-${run_id}.env"

github_user="${GITHUB_USER:-}"
github_actor="${GITHUB_ACTOR:-}"
github_repository="${GITHUB_REPOSITORY:-}"
codespaces_flag="${CODESPACES:-}"
user_value="${USER:-}"
logname_value="${LOGNAME:-}"

sanitize_field() {
  local value="$1"
  value="${value//|/_}"
  value="${value//$'\n'/ }"
  value="${value//$'\r'/ }"
  printf '%s' "$value"
}

print_selected_env() {
  local key
  for key in \
    GITHUB_USER \
    GITHUB_ACTOR \
    GITHUB_REPOSITORY \
    CODESPACES \
    CODESPACE_NAME \
    USER \
    LOGNAME \
    HOME \
    PWD \
    PHASE_EVAL_RUN_ID \
    PHASE_EVAL_SCENARIO; do
    printf '%s=%q\n' "$key" "${!key-}"
  done
}

prebuild_update_marker_before_call="false"
if [[ -f "$prebuild_update_marker_path" ]]; then
  prebuild_update_marker_before_call="true"
fi

persistent_history_before_call="false"
if [[ -s "$persistent_history_path" ]]; then
  persistent_history_before_call="true"
fi

phase_bucket="$(resolve_phase_bucket "$hook_name" "$persistent_history_before_call" "$prebuild_update_marker_before_call")"
phase_detail="$(resolve_phase_detail)"
mkdir -p "$persistent_root/markers/$phase_bucket"
persistent_marker_path="$persistent_root/markers/$phase_bucket/${hook_name}.marker"
persistent_run_marker_path="$persistent_root/markers/$phase_bucket/${hook_name}-${run_id}.marker"

print_selected_env >"$volatile_env_path"
print_selected_env >"$persistent_env_path"
touch "$volatile_marker_path" "$persistent_marker_path" "$persistent_run_marker_path"

event_line="PHASE_EVAL_EVENT|hook=$(sanitize_field "$hook_name")|phase_bucket=$(sanitize_field "$phase_bucket")|phase_detail=$(sanitize_field "$phase_detail")|run_id=$(sanitize_field "$run_id")|scenario=$(sanitize_field "$scenario")|timestamp=$timestamp|persistent_history_before_call=$(sanitize_field "$persistent_history_before_call")|prebuild_update_marker_before_call=$(sanitize_field "$prebuild_update_marker_before_call")|github_user=$(sanitize_field "$github_user")|github_actor=$(sanitize_field "$github_actor")|github_repository=$(sanitize_field "$github_repository")|codespaces=$(sanitize_field "$codespaces_flag")|user=$(sanitize_field "$user_value")|logname=$(sanitize_field "$logname_value")"

printf '%s\n' "$event_line" | tee -a "$volatile_root/events.log"
printf '%s\n' "$event_line" >>"$persistent_history_path"
