#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/../devpod-local/lib.sh"

# Intent: Validate decomk behavior parity in GitHub Codespaces by creating a
# fresh codespace from the pushed branch under test, running production stage-0
# bootstrap with explicit URI inputs, and asserting the same fixture markers
# used by the local DevPod harness.
# Source: DI-007-20260412-230000 (TODO/007)

usage() {
  cat <<'USAGE'
Usage:
  examples/decomk-selftest/codespaces/run.sh [options] [decomk run args...]

Required options:
  --conf-uri <git:...>       Remote-reachable fixture config repo URI.

Optional options:
  --repo <owner/repo>        GitHub repo slug (default: parsed from origin URL).
  --branch <name>            Branch to test (default: current branch).
  --tool-uri <uri>           Tool source URI (default: git:https://github.com/<repo>.git?ref=<branch>).
  --devcontainer-path <path> Repo-relative devcontainer.json path used for codespace create.
                             (default: examples/decomk-selftest/codespaces/.devcontainer/devcontainer.json)
  --idle-timeout <dur>       Codespace inactivity timeout (default: 30m).
  --retention-period <dur>   Codespace retention period after stop (default: 1h).
  --create-timeout <sec>     Wait timeout for codespace discovery (default: 900).
  --ssh-timeout <sec>        Wait timeout for SSH readiness (default: 600).
  --name-prefix <prefix>     Display-name prefix (default: dst-codespaces).
  --keep-on-fail             Keep failed codespace for debugging.
  -h, --help                 Show this message.

Examples:
  examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/example/confrepo.git
  examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/example/confrepo.git all
  examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/example/confrepo.git TUPLE_VERIFY_TOOL TUPLE_VERIFY_CONF TUPLE_CONTEXT_OVERRIDE TUPLE_DEFAULT_SHARED
USAGE
}

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

fail() {
  local code="$1"
  shift
  printf '[decomk-selftest] ERROR: %s\n' "$*" >&2
  exit "$code"
}

infer_repo_slug_from_origin() {
  local origin_url
  if ! origin_url="$(git remote get-url origin 2>/dev/null)"; then
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
  if [[ -z "$owner" || -z "$repo_name" ]]; then
    return 1
  fi

  printf '%s/%s\n' "$owner" "$repo_name"
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
  local timeout_seconds="$1"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    if gh codespace ssh -c "$codespace_name" -- "true" >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done

  return 1
}

codespace_bash() {
  local script="$1"
  gh codespace ssh -c "$codespace_name" -- "bash -lc $(printf '%q' "$script")"
}

latest_make_log_path() {
  codespace_bash "find /tmp/decomk-selftest/log -type f -name make.log | sort | tail -n1"
}

load_make_log_or_fail() {
  local exit_code="$1"
  local log_path="$2"
  local log_content

  if ! log_content="$(codespace_bash "cat $(printf '%q' "$log_path")")"; then
    fail "$exit_code" "failed to read make.log at $log_path"
  fi

  mapfile -t make_log_lines <<<"$log_content"
}

has_marker() {
  local marker="$1"
  local line
  for line in "${make_log_lines[@]}"; do
    if [[ "$line" == *"$marker"* ]]; then
      return 0
    fi
  done
  return 1
}

require_marker_or_fail() {
  local exit_code="$1"
  local marker="$2"
  if ! has_marker "$marker"; then
    fail "$exit_code" "required marker not found in make.log: $marker"
  fi
}

require_absent_marker_or_fail() {
  local exit_code="$1"
  local marker="$2"
  if has_marker "$marker"; then
    fail "$exit_code" "unexpected marker found in make.log: $marker"
  fi
}

require_no_fail_markers_or_fail() {
  local exit_code="$1"
  local line
  for line in "${make_log_lines[@]}"; do
    if [[ "$line" == *"SELFTEST FAIL"* ]]; then
      fail "$exit_code" "failure marker found in make.log: $line"
    fi
  done
}

repo_root="$(cd "$script_dir/../../.." && pwd)"

repo_slug=""
branch=""
conf_uri=""
tool_uri=""
devcontainer_path="examples/decomk-selftest/codespaces/.devcontainer/devcontainer.json"
idle_timeout="30m"
retention_period="1h"
create_timeout=900
ssh_timeout=600
name_prefix="dst-codespaces"
keep_on_fail="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      [[ $# -ge 2 ]] || fail 2 "--repo requires a value"
      repo_slug="$2"
      shift 2
      ;;
    --branch)
      [[ $# -ge 2 ]] || fail 2 "--branch requires a value"
      branch="$2"
      shift 2
      ;;
    --conf-uri)
      [[ $# -ge 2 ]] || fail 2 "--conf-uri requires a value"
      conf_uri="$2"
      shift 2
      ;;
    --tool-uri)
      [[ $# -ge 2 ]] || fail 2 "--tool-uri requires a value"
      tool_uri="$2"
      shift 2
      ;;
    --devcontainer-path)
      [[ $# -ge 2 ]] || fail 2 "--devcontainer-path requires a value"
      devcontainer_path="$2"
      shift 2
      ;;
    --idle-timeout)
      [[ $# -ge 2 ]] || fail 2 "--idle-timeout requires a value"
      idle_timeout="$2"
      shift 2
      ;;
    --retention-period)
      [[ $# -ge 2 ]] || fail 2 "--retention-period requires a value"
      retention_period="$2"
      shift 2
      ;;
    --create-timeout)
      [[ $# -ge 2 ]] || fail 2 "--create-timeout requires a value"
      create_timeout="$2"
      shift 2
      ;;
    --ssh-timeout)
      [[ $# -ge 2 ]] || fail 2 "--ssh-timeout requires a value"
      ssh_timeout="$2"
      shift 2
      ;;
    --name-prefix)
      [[ $# -ge 2 ]] || fail 2 "--name-prefix requires a value"
      name_prefix="$2"
      shift 2
      ;;
    --keep-on-fail)
      keep_on_fail="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      fail 2 "unknown option: $1"
      ;;
    *)
      break
      ;;
  esac
done

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
  if [[ "$arg" == -* ]]; then
    fail 2 "run.sh accepts decomk action args only (flags such as $arg are not allowed)"
  fi
done

if [[ -z "$conf_uri" ]]; then
  fail 2 "--conf-uri is required"
fi
if [[ "$conf_uri" != git:* ]]; then
  fail 2 "--conf-uri must be a git: URI"
fi

if ! [[ "$create_timeout" =~ ^[0-9]+$ ]]; then
  fail 2 "--create-timeout must be an integer number of seconds"
fi
if ! [[ "$ssh_timeout" =~ ^[0-9]+$ ]]; then
  fail 2 "--ssh-timeout must be an integer number of seconds"
fi

if [[ "$devcontainer_path" = /* ]]; then
  fail 2 "--devcontainer-path must be repo-relative, not absolute"
fi
if [[ ! -f "$repo_root/$devcontainer_path" ]]; then
  fail 2 "devcontainer file not found at $repo_root/$devcontainer_path"
fi

need_command gh
need_command git

if ! gh auth status -t >/dev/null 2>&1; then
  fail 10 "GitHub CLI is not authenticated; run: gh auth login"
fi

if [[ -z "$repo_slug" ]]; then
  if ! repo_slug="$(infer_repo_slug_from_origin)"; then
    fail 10 "unable to infer --repo from origin URL; set --repo <owner/repo>"
  fi
fi

if [[ -z "$branch" ]]; then
  branch="$(git rev-parse --abbrev-ref HEAD)"
fi
if [[ "$branch" == "HEAD" || -z "$branch" ]]; then
  fail 2 "detached HEAD; set --branch explicitly"
fi

local_head="$(git rev-parse HEAD)"
if ! remote_line="$(git ls-remote --exit-code origin "refs/heads/$branch" 2>/dev/null)"; then
  fail 11 "remote branch origin/$branch not found; commit and push first"
fi
remote_head="${remote_line%%[[:space:]]*}"
if [[ "$local_head" != "$remote_head" ]]; then
  fail 11 "local HEAD ($local_head) does not match origin/$branch ($remote_head); commit and push first"
fi

if [[ -z "$tool_uri" ]]; then
  tool_uri="git:https://github.com/$repo_slug.git?ref=$branch"
fi

decomk_run_args="$(join_space_args "${decomk_args[@]}")"
if [[ -z "$decomk_run_args" ]]; then
  fail 2 "resolved DECOMK_RUN_ARGS is empty"
fi

run_id="$(date -u +%Y%m%d-%H%M%S)-$$"
display_name="${name_prefix}-${run_id}"
display_name="${display_name:0:48}"
if [[ -z "$display_name" ]]; then
  fail 2 "display name resolved to empty value"
fi

temp_root="$(mktemp -d /tmp/decomk-codespaces.XXXXXX)"
codespace_name=""
codespace_ready="false"
make_log_lines=()

cleanup() {
  local exit_code=$?
  set +e

  if [[ -n "$codespace_name" ]]; then
    log "collecting diagnostics: $temp_root"
    gh codespace logs -c "$codespace_name" >"$temp_root/codespace.log" 2>&1 || true

    if [[ "$codespace_ready" == "true" ]]; then
      codespace_bash "find /tmp/decomk-selftest/log -type f -name make.log | sort" >"$temp_root/make-log-paths.txt" 2>/dev/null || true
      gh codespace cp -c "$codespace_name" -r "remote:/tmp/decomk-selftest/log" "$temp_root/remote-log-dir" >/dev/null 2>&1 || true
    fi

    if [[ "$exit_code" -ne 0 && "$keep_on_fail" == "true" ]]; then
      log "keeping failed codespace for debugging: $codespace_name"
    else
      gh codespace stop -c "$codespace_name" >/dev/null 2>&1 || true
      gh codespace delete -c "$codespace_name" -f >/dev/null 2>&1 || true
    fi
  fi

  if [[ "$exit_code" -eq 0 ]]; then
    rm -rf "$temp_root"
  else
    log "failure artifacts preserved at: $temp_root"
  fi
}
trap cleanup EXIT

log "repo: $repo_slug"
log "branch: $branch"
log "devcontainer path: $devcontainer_path"
log "conf URI: $conf_uri"
log "tool URI: $tool_uri"
log "run args: $decomk_run_args"
log "codespace display name: $display_name"

run_logged gh codespace create \
  -R "$repo_slug" \
  -b "$branch" \
  --devcontainer-path "$devcontainer_path" \
  -d "$display_name" \
  --idle-timeout "$idle_timeout" \
  --retention-period "$retention_period" \
  --default-permissions

if ! codespace_name="$(resolve_codespace_name "$repo_slug" "$display_name" "$create_timeout")"; then
  fail 21 "timed out waiting to discover created codespace by display name: $display_name"
fi
log "codespace name: $codespace_name"

if ! wait_for_codespace_ssh "$ssh_timeout"; then
  fail 22 "timed out waiting for SSH readiness on codespace: $codespace_name"
fi
codespace_ready="true"

repo_basename="${repo_slug##*/}"
repo_basename_q="$(printf '%q' "$repo_basename")"
decomk_home_q="$(printf '%q' "/tmp/decomk-selftest/home")"
decomk_log_dir_q="$(printf '%q' "/tmp/decomk-selftest/log")"
tool_uri_q="$(printf '%q' "$tool_uri")"
conf_uri_q="$(printf '%q' "$conf_uri")"
run_args_q="$(printf '%q' "$decomk_run_args")"

remote_stage0_script="$(cat <<EOF
set -euo pipefail
workspace_dir="/workspaces/$repo_basename"
if [[ ! -f "\$workspace_dir/examples/devcontainer/postCreateCommand.sh" ]]; then
  workspace_dir="\$(find /workspaces -mindepth 1 -maxdepth 2 -type d -name $repo_basename_q | head -n1 || true)"
fi
if [[ -z "\$workspace_dir" ]] || [[ ! -f "\$workspace_dir/examples/devcontainer/postCreateCommand.sh" ]]; then
  echo "unable to locate workspace root containing examples/devcontainer/postCreateCommand.sh" >&2
  exit 1
fi
cd "\$workspace_dir"
export DECOMK_HOME=$decomk_home_q
export DECOMK_LOG_DIR=$decomk_log_dir_q
export DECOMK_TOOL_URI=$tool_uri_q
export DECOMK_CONF_URI=$conf_uri_q
export DECOMK_RUN_ARGS=$run_args_q
bash examples/devcontainer/postCreateCommand.sh
EOF
)"

if ! run_logged codespace_bash "$remote_stage0_script"; then
  fail 30 "stage-0 bootstrap run failed in codespace"
fi

make_log_path="$(latest_make_log_path)"
if [[ -z "$make_log_path" ]]; then
  fail 31 "could not find make.log under /tmp/decomk-selftest/log"
fi
log "using make log: $make_log_path"
load_make_log_or_fail 31 "$make_log_path"
require_no_fail_markers_or_fail 31
require_marker_or_fail 31 "SELFTEST PASS conf-repo-origin"
require_marker_or_fail 31 "SELFTEST PASS tool-repo-origin"
require_marker_or_fail 31 "SELFTEST PASS context-override"
require_marker_or_fail 31 "SELFTEST PASS default-tuple-available"

if ! run_logged codespace_bash "decomk run TUPLE_STAMP_PROBE"; then
  fail 32 "stamp probe run failed"
fi
stamp_probe_log_path="$(latest_make_log_path)"
if [[ -z "$stamp_probe_log_path" ]]; then
  fail 32 "could not find stamp-probe make.log"
fi
load_make_log_or_fail 32 "$stamp_probe_log_path"
require_no_fail_markers_or_fail 32
require_marker_or_fail 32 "SELFTEST PASS stamp-dir-working-dir"
require_marker_or_fail 32 "SELFTEST PASS stamp-probe-ran"

if ! run_logged codespace_bash "decomk run TUPLE_STAMP_PROBE TUPLE_STAMP_VERIFY"; then
  fail 32 "stamp verify run failed"
fi
stamp_verify_log_path="$(latest_make_log_path)"
if [[ -z "$stamp_verify_log_path" ]]; then
  fail 32 "could not find stamp-verify make.log"
fi
if [[ "$stamp_verify_log_path" == "$stamp_probe_log_path" ]]; then
  fail 32 "stamp runs reused the same make.log path unexpectedly: $stamp_verify_log_path"
fi
load_make_log_or_fail 32 "$stamp_verify_log_path"
require_no_fail_markers_or_fail 32
require_marker_or_fail 32 "SELFTEST PASS stamp-idempotent"
require_absent_marker_or_fail 32 "SELFTEST PASS stamp-probe-ran"

log "codespaces parity checks passed"
