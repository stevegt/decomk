#!/usr/bin/env bash

set -euo pipefail

hook_name="${1:-}"
if [[ -z "$hook_name" ]]; then
  echo "phase-eval hook probe: missing hook name argument" >&2
  exit 2
fi

run_id="${PHASE_EVAL_RUN_ID:-${CODESPACE_NAME:-manual}}"
scenario="${PHASE_EVAL_SCENARIO:-unknown}"

# Intent: Persist hook observations in a stable in-container location so both
# build logs and post-start SSH probes can verify lifecycle behavior empirically.
# Source: DI-009-20260413-232813 (TODO/009)
root_dir="/tmp/decomk-phase-eval-hooks"
mkdir -p "$root_dir/env" "$root_dir/markers"

timestamp="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
env_path="$root_dir/env/${hook_name}-${run_id}.env"
marker_path="$root_dir/markers/${hook_name}-${run_id}.marker"

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

print_selected_env >"$env_path"
touch "$marker_path"

event_line="PHASE_EVAL_EVENT|hook=$(sanitize_field "$hook_name")|run_id=$(sanitize_field "$run_id")|scenario=$(sanitize_field "$scenario")|timestamp=$timestamp|github_user=$(sanitize_field "$github_user")|github_actor=$(sanitize_field "$github_actor")|github_repository=$(sanitize_field "$github_repository")|codespaces=$(sanitize_field "$codespaces_flag")|user=$(sanitize_field "$user_value")|logname=$(sanitize_field "$logname_value")"

printf '%s\n' "$event_line" | tee -a "$root_dir/events.log"
