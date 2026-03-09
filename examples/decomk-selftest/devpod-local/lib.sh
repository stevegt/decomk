#!/usr/bin/env bash

set -euo pipefail

# Intent: Keep harness scripts focused on scenario logic by centralizing logging,
# dependency checks, and DevPod workspace cleanup in one shared helper module.
# Source: DI-007-20260309-124345 (TODO/007)

log() {
  printf '[decomk-selftest] %s\n' "$*"
}

die() {
  printf '[decomk-selftest] ERROR: %s\n' "$*" >&2
  exit 1
}

need_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    die "missing required command: $command_name"
  fi
}

run_logged() {
  log "+ $*"
  "$@"
}

ensure_docker_provider_selected() {
  # Intent: Force deterministic provider selection so the harness always validates
  # DevPod's local Docker-provider path rather than whichever provider was last used.
  # Source: DI-007-20260309-124345 (TODO/007)
  if ! devpod provider use docker >/dev/null 2>&1; then
    log "docker provider not active; attempting to add it"
    run_logged devpod provider add docker
    run_logged devpod provider use docker
  else
    log "docker provider selected"
  fi
}

cleanup_workspace() {
  local workspace_name="$1"
  if [[ -z "$workspace_name" ]]; then
    return 0
  fi
  devpod delete "$workspace_name" >/dev/null 2>&1 || true
}
