#!/usr/bin/env bash

set -euo pipefail

# Intent: Keep phase-eval scenario scripts focused on platform-specific evaluation
# logic by centralizing logging, command checks, and rc/stdout/stderr capture.
# Source: DI-009-20260413-232813 (TODO/009)

log() {
  printf '[phase-eval] %s\n' "$*"
}

die() {
  printf '[phase-eval] ERROR: %s\n' "$*" >&2
  exit 1
}

need_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    die "missing required command: $command_name"
  fi
}

run_capture() {
  local out_dir="$1"
  local step_name="$2"
  shift 2

  local raw_dir="$out_dir/raw"
  local stdout_path="$raw_dir/${step_name}.stdout.log"
  local stderr_path="$raw_dir/${step_name}.stderr.log"
  local rc_path="$raw_dir/${step_name}.rc"
  mkdir -p "$raw_dir"

  log "+ $*"

  local rc=0
  if "$@" >"$stdout_path" 2>"$stderr_path"; then
    rc=0
  else
    rc=$?
  fi

  printf '%s\n' "$rc" >"$rc_path"
  return "$rc"
}

append_scenario_note() {
  local out_dir="$1"
  local scenario="$2"
  local message="$3"
  printf '%s\t%s\n' "$scenario" "$message" >>"$out_dir/scenario-notes.tsv"
}

sanitize_json_string() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/ }"
  value="${value//$'\r'/ }"
  value="${value//$'\t'/ }"
  printf '%s' "$value"
}
