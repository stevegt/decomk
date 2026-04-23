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

Optional options:
  --repo <owner/repo>        GitHub repo slug (default: parsed from origin URL).
  --branch <name>            Branch to test (default: current branch).
  --conf-uri <git:...>       Optional override for config source URI.
                             Default: auto-generated fixture repo inside codespace.
  --tool-uri <uri>           Tool source URI (default: git:https://github.com/<repo>.git?ref=<branch>).
  --machine <name>           Codespace machine type override.
                             Default: auto-select from repository-allowed machine list
                             (prefers basicLinux32gb when available).
  --devcontainer-path <path> Repo-relative devcontainer.json path used for codespace create.
                             (default: .devcontainer/codespaces-selftest/devcontainer.json)
  --idle-timeout <dur>       Codespace inactivity timeout (default: 30m).
  --retention-period <dur>   Codespace retention period after stop (default: 1h).
  --create-timeout <sec>     Wait timeout for codespace discovery (default: 900).
  --ssh-timeout <sec>        Wait timeout for SSH readiness (default: 600).
  --name-prefix <prefix>     Display-name prefix (default: dst-codespaces).
  --keep-on-fail             Keep failed codespace for debugging.
  --cleanup                  Remove local /tmp harness artifacts on success.
                             Default keeps artifacts for post-run inspection.
  -h, --help                 Show this message.

Examples:
  examples/decomk-selftest/codespaces/run.sh
  examples/decomk-selftest/codespaces/run.sh all
  examples/decomk-selftest/codespaces/run.sh TUPLE_VERIFY_TOOL TUPLE_VERIFY_CONF TUPLE_CONTEXT_OVERRIDE TUPLE_DEFAULT_SHARED
  examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/example/confrepo.git
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

resolve_machine_name() {
  local requested_machine="$1"
  local repo="$2"
  local available_names=()
  local available_name
  local candidate
  local machine_lines=""

  # Intent: Keep codespaces harness non-interactive by resolving machine choice
  # from repository-allowed machine lists before create calls, and fail fast
  # when discovery endpoints are unavailable instead of passing malformed values
  # into `gh codespace create`.
  # Source: DI-007-20260413-040500 (TODO/007)
  if machine_lines="$(gh api "repos/$repo/codespaces/machines" --jq '.machines[].name' 2>/dev/null)"; then
    :
  elif machine_lines="$(gh api /user/codespaces/machines --jq '.machines[].name' 2>/dev/null)"; then
    :
  else
    return 1
  fi

  if [[ -z "$machine_lines" ]]; then
    return 1
  fi
  mapfile -t available_names <<<"$machine_lines"
  local compact_names=()
  for available_name in "${available_names[@]}"; do
    if [[ -n "$available_name" ]]; then
      compact_names+=("$available_name")
    fi
  done
  available_names=("${compact_names[@]}")
  if [[ ${#available_names[@]} -eq 0 ]]; then
    return 1
  fi

  if [[ -n "$requested_machine" ]]; then
    for candidate in "${available_names[@]}"; do
      if [[ "$candidate" == "$requested_machine" ]]; then
        printf '%s\n' "$candidate"
        return 0
      fi
    done
    return 2
  fi

  for candidate in basicLinux32gb standardLinux32gb basicLinux64gb standardLinux64gb; do
    for available_name in "${available_names[@]}"; do
      if [[ "$available_name" == "$candidate" ]]; then
        printf '%s\n' "$available_name"
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
  local wait_ticks=0

  while ((SECONDS < deadline)); do
    while IFS=$'\t' read -r listed_display listed_name; do
      if [[ "$listed_display" == "$display_name" && -n "$listed_name" ]]; then
        if (( wait_ticks > 0 )); then
          printf '\n' >&2
        fi
        printf '%s\n' "$listed_name"
        return 0
      fi
    done < <(gh codespace list -R "$repo" --json displayName,name --jq '.[] | [.displayName, .name] | @tsv')
    wait_ticks=$((wait_ticks + 1))
    emit_wait_progress "codespace discovery" "$wait_ticks" "$((wait_ticks * 5))"
    sleep 5
  done

  if (( wait_ticks > 0 )); then
    printf '\n' >&2
  fi
  return 1
}

wait_for_codespace_ssh() {
  local timeout_seconds="$1"
  local deadline=$((SECONDS + timeout_seconds))
  local wait_ticks=0

  while ((SECONDS < deadline)); do
    if gh codespace ssh -c "$codespace_name" -- "true" >/dev/null 2>&1; then
      if (( wait_ticks > 0 )); then
        printf '\n' >&2
      fi
      return 0
    fi
    wait_ticks=$((wait_ticks + 1))
    emit_wait_progress "codespace SSH readiness" "$wait_ticks" "$((wait_ticks * 5))"
    sleep 5
  done

  if (( wait_ticks > 0 )); then
    printf '\n' >&2
  fi
  return 1
}

emit_wait_progress() {
  local wait_label="$1"
  local wait_ticks="$2"
  local elapsed_seconds="$3"

  # Intent: Show visible progress while waiting on slow Codespaces startup
  # phases so operators can distinguish normal startup latency from a hung run.
  # Source: DI-007-20260413-043500 (TODO/007)
  if (( wait_ticks == 1 )); then
    printf '[decomk-selftest] waiting for %s' "$wait_label" >&2
  fi
  printf '.' >&2
  if (( wait_ticks % 12 == 0 )); then
    printf ' %ss\n[decomk-selftest] waiting for %s' "$elapsed_seconds" "$wait_label" >&2
  fi
}

codespace_bash() {
  local script="$1"
  gh codespace ssh -c "$codespace_name" -- "bash -lc $(printf '%q' "$script")"
}

latest_make_log_path() {
  local decomk_log_dir_q
  decomk_log_dir_q="$(printf '%q' "$decomk_log_dir")"
  codespace_bash "find $decomk_log_dir_q -type f -name make.log | sort | tail -n1"
}

sanitize_diag_step_name() {
  local step_name="$1"
  step_name="${step_name//[^a-zA-Z0-9._-]/_}"
  printf '%s' "$step_name"
}

record_diag_step() {
  local step_name="$1"
  shift

  local safe_step_name
  safe_step_name="$(sanitize_diag_step_name "$step_name")"
  local stdout_path="$temp_root/diag-${safe_step_name}.stdout.log"
  local stderr_path="$temp_root/diag-${safe_step_name}.stderr.log"
  local rc_path="$temp_root/diag-${safe_step_name}.rc"

  local rc=0
  if "$@" >"$stdout_path" 2>"$stderr_path"; then
    rc=0
  else
    rc=$?
  fi

  printf '%s\n' "$rc" >"$rc_path"
  if [[ "$rc" -eq 0 ]]; then
    printf '%s\tOK\t0\n' "$step_name" >>"$diag_summary_path"
    return 0
  fi

  printf '%s\tFAIL\t%s\n' "$step_name" "$rc" >>"$diag_summary_path"
  log "diagnostics step failed: $step_name (rc=$rc; stderr: $stderr_path)"
  return "$rc"
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
devcontainer_path=".devcontainer/codespaces-selftest/devcontainer.json"
machine=""
idle_timeout="30m"
retention_period="1h"
create_timeout=900
ssh_timeout=600
name_prefix="dst-codespaces"
keep_on_fail="false"
cleanup_on_success="false"
decomk_home="/tmp/decomk-selftest/home"
decomk_log_dir="/tmp/decomk-selftest/log"

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
    --machine)
      [[ $# -ge 2 ]] || fail 2 "--machine requires a value"
      machine="$2"
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
    --cleanup)
      cleanup_on_success="true"
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

if [[ -n "$conf_uri" ]] && [[ "$conf_uri" != git:* ]]; then
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
# Intent: Fail fast on non-Codespaces-compatible devcontainer paths so create
# requests don't make network calls that are known to fail. Codespaces create
# accepts paths rooted under `.devcontainer/`.
# Source: DI-007-20260413-011500 (TODO/007)
if [[ "$devcontainer_path" != .devcontainer/* ]]; then
  fail 2 "--devcontainer-path must live under .devcontainer/ for Codespaces create API compatibility"
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

machine_resolution_status=0
resolved_machine=""
resolved_machine="$(resolve_machine_name "$machine" "$repo_slug")" || machine_resolution_status=$?
if [[ "$machine_resolution_status" -ne 0 ]]; then
  case "$machine_resolution_status" in
    1) fail 10 "unable to resolve a codespace machine from API; pass --machine explicitly or check gh/api access" ;;
    2) fail 2 "requested --machine is not in the allowed machine list for this repo; run: gh api repos/$repo_slug/codespaces/machines --jq '.machines[] | [.name, .displayName] | @tsv'" ;;
    *) fail 10 "failed to resolve codespace machine type" ;;
  esac
fi
machine="$resolved_machine"

decomk_run_args="$(join_space_args "${decomk_args[@]}")"
if [[ -z "$decomk_run_args" ]]; then
  fail 2 "resolved decomk action args are empty"
fi

run_id="$(date -u +%Y%m%d-%H%M%S)-$$"
display_name="${name_prefix}-${run_id}"
display_name="${display_name:0:48}"
if [[ -z "$display_name" ]]; then
  fail 2 "display name resolved to empty value"
fi

temp_root="$(mktemp -d /tmp/decomk-codespaces.XXXXXX)"
diag_summary_path="$temp_root/diagnostics-summary.txt"
codespace_name=""
codespace_ready="false"
make_log_lines=()

cleanup() {
  local exit_code=$?
  set +e
  local diagnostics_failed="false"

  : >"$diag_summary_path"
  printf 'run_exit_code\t%s\n' "$exit_code" >>"$diag_summary_path"
  printf 'codespace_name\t%s\n' "$codespace_name" >>"$diag_summary_path"

  if [[ -n "$codespace_name" ]]; then
    log "collecting diagnostics: $temp_root"
    if ! record_diag_step "codespace-logs" gh codespace logs -c "$codespace_name"; then
      diagnostics_failed="true"
    fi
    if ! cp "$temp_root/diag-codespace-logs.stdout.log" "$temp_root/codespace.log"; then
      log "diagnostics step failed: persist-codespace-log-copy (stderr: $temp_root/diag-codespace-logs.stderr.log)"
      diagnostics_failed="true"
    fi

    if [[ "$codespace_ready" == "true" ]]; then
      local decomk_log_dir_q
      decomk_log_dir_q="$(printf '%q' "$decomk_log_dir")"
      if record_diag_step "list-make-log-paths" codespace_bash "find $decomk_log_dir_q -type f -name make.log | sort"; then
        if ! cp "$temp_root/diag-list-make-log-paths.stdout.log" "$temp_root/make-log-paths.txt"; then
          log "diagnostics step failed: persist-make-log-path-copy"
          diagnostics_failed="true"
        fi
      else
        : >"$temp_root/make-log-paths.txt"
        diagnostics_failed="true"
      fi

      if ! record_diag_step "copy-remote-log-dir" gh codespace cp -c "$codespace_name" -r "remote:$decomk_log_dir" "$temp_root/remote-log-dir"; then
        diagnostics_failed="true"
      fi
    fi

    if [[ "$exit_code" -ne 0 && "$keep_on_fail" == "true" ]]; then
      log "keeping failed codespace for debugging: $codespace_name"
      printf 'codespace-cleanup\tSKIPPED\tkeep-on-fail\n' >>"$diag_summary_path"
    else
      if ! record_diag_step "codespace-stop" gh codespace stop -c "$codespace_name"; then
        diagnostics_failed="true"
      fi
      if ! record_diag_step "codespace-delete" gh codespace delete -c "$codespace_name" -f; then
        diagnostics_failed="true"
      fi
    fi
  else
    printf 'codespace\tSKIPPED\tmissing-name\n' >>"$diag_summary_path"
  fi

  # Intent: Record teardown command status in explicit diagnostics files so
  # selftest runs never hide cleanup or artifact-capture failures.
  # Source: DI-008-20260412-122157 (TODO/008)
  if [[ "$diagnostics_failed" == "true" ]]; then
    log "diagnostics capture had failures; see $diag_summary_path"
  fi
  printf 'complete\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$temp_root/diagnostics.complete"

  # Intent: Preserve local harness artifacts by default so successful runs are
  # inspectable after completion; only delete them when operators explicitly
  # opt into cleanup.
  # Source: DI-007-20260412-042700 (TODO/007)
  if [[ "$exit_code" -ne 0 ]]; then
    log "failure artifacts preserved at: $temp_root"
  elif [[ "$cleanup_on_success" == "true" ]]; then
    if ! rm -rf "$temp_root"; then
      log "cleanup warning: failed to remove $temp_root"
      log "artifacts preserved at: $temp_root"
    fi
  else
    log "artifacts preserved at: $temp_root"
  fi
}
trap cleanup EXIT

log "repo: $repo_slug"
log "branch: $branch"
log "machine: $machine"
log "devcontainer path: $devcontainer_path"
if [[ -n "$conf_uri" ]]; then
  log "conf URI override: $conf_uri"
else
  log "conf URI: auto-generated fixture repo in codespace"
fi
log "tool URI: $tool_uri"
log "run args: $decomk_run_args"
log "codespace display name: $display_name"

run_logged gh codespace create \
  -R "$repo_slug" \
  -b "$branch" \
  --machine "$machine" \
  --devcontainer-path "$devcontainer_path" \
  -d "$display_name" \
  --idle-timeout "$idle_timeout" \
  --retention-period "$retention_period" \
  --default-permissions

if ! codespace_name="$(resolve_codespace_name "$repo_slug" "$display_name" "$create_timeout")"; then
  fail 21 "timed out waiting to discover created codespace by display name: $display_name"
fi
log "codespace name: $codespace_name"

# Intent: The harness executes all remote checks via `gh codespace ssh`, so the
# selftest devcontainer must include an SSH server feature. Keep the failure
# message explicit so stale codespaces or feature drift are actionable.
# Source: DI-007-20260413-051500 (TODO/007)
if ! wait_for_codespace_ssh "$ssh_timeout"; then
  fail 22 "timed out waiting for SSH readiness on codespace: $codespace_name (verify selftest devcontainer includes ghcr.io/devcontainers/features/sshd:1 and recreate the codespace)"
fi
codespace_ready="true"

# Intent: Keep the Codespaces harness self-contained by synthesizing the config
# fixture repo inside the Codespace when no `--conf-uri` override is supplied.
# This removes external fixture hosting requirements while still forcing stage-0
# bootstrap to exercise git clone/pull from a URI source.
# Source: DI-007-20260413-000500 (TODO/007)
repo_basename="${repo_slug##*/}"
repo_basename_q="$(printf '%q' "$repo_basename")"
run_id_q="$(printf '%q' "$run_id")"
decomk_home_q="$(printf '%q' "$decomk_home")"
decomk_log_dir_q="$(printf '%q' "$decomk_log_dir")"
tool_uri_q="$(printf '%q' "$tool_uri")"
conf_uri_override_q="$(printf '%q' "$conf_uri")"
decomk_run_args_shell=""
for arg in "${decomk_args[@]}"; do
  decomk_run_args_shell+=" $(printf '%q' "$arg")"
done

remote_stage0_script="$(cat <<EOF
set -euo pipefail
workspace_dir="/workspaces/$repo_basename"
if [[ ! -f "\$workspace_dir/examples/devcontainer/decomk-stage0.sh" ]]; then
  if ! workspace_dir="\$(find /workspaces -mindepth 1 -maxdepth 2 -type d -name $repo_basename_q | head -n1)"; then
    echo "failed searching /workspaces for repo $repo_basename" >&2
    exit 1
  fi
fi
if [[ -z "\$workspace_dir" ]] || [[ ! -f "\$workspace_dir/examples/devcontainer/decomk-stage0.sh" ]]; then
  echo "unable to locate workspace root containing examples/devcontainer/decomk-stage0.sh" >&2
  exit 1
fi
cd "\$workspace_dir"
conf_uri_override=$conf_uri_override_q
if [[ -n "\$conf_uri_override" ]]; then
  resolved_conf_uri="\$conf_uri_override"
else
  fixture_root="/tmp/decomk-codespaces-fixture/$run_id_q"
  conf_work="\$fixture_root/confrepo-work"
  conf_bare="\$fixture_root/confrepo.git"
  rm -rf "\$fixture_root"
  mkdir -p "\$conf_work"
  cp -a examples/decomk-selftest/fixtures/confrepo/. "\$conf_work"
  git -C "\$conf_work" init -q
  git -C "\$conf_work" config user.name "decomk selftest"
  git -C "\$conf_work" config user.email "selftest@example.invalid"
  git -C "\$conf_work" add .
  git -C "\$conf_work" commit -q -m "Seed decomk codespaces fixture repo"
  git clone -q --bare "\$conf_work" "\$conf_bare"
  resolved_conf_uri="git:file://\$conf_bare"
fi
export DECOMK_HOME=$decomk_home_q
export DECOMK_LOG_DIR=$decomk_log_dir_q
export DECOMK_TOOL_URI=$tool_uri_q
export DECOMK_CONF_URI="\$resolved_conf_uri"
bash examples/devcontainer/decomk-stage0.sh postCreate$decomk_run_args_shell
EOF
)"

if ! run_logged codespace_bash "$remote_stage0_script"; then
  fail 30 "stage-0 bootstrap run failed in codespace"
fi

make_log_path="$(latest_make_log_path)"
if [[ -z "$make_log_path" ]]; then
  fail 31 "could not find make.log under $decomk_log_dir"
fi
log "using make log: $make_log_path"
load_make_log_or_fail 31 "$make_log_path"
require_no_fail_markers_or_fail 31
require_marker_or_fail 31 "SELFTEST PASS conf-repo-origin"
require_marker_or_fail 31 "SELFTEST PASS tool-repo-origin"
require_marker_or_fail 31 "SELFTEST PASS context-override"
require_marker_or_fail 31 "SELFTEST PASS default-tuple-available"

run_decomk_with_stage0_env() {
  local decomk_run_action_args="$1"

  # Intent: Keep post-bootstrap `decomk run` invocations on the same DECOMK_HOME
  # and DECOMK_LOG_DIR used during stage-0 bootstrap, so stamp regression checks
  # operate on the same config/stamp/log state instead of falling back to /var paths.
  # Source: DI-007-20260413-053500 (TODO/007)
  local run_script
  run_script="$(cat <<EOF
set -euo pipefail
workspace_dir="/workspaces/$repo_basename"
if [[ ! -d "\$workspace_dir" ]]; then
  if ! workspace_dir="\$(find /workspaces -mindepth 1 -maxdepth 2 -type d -name $repo_basename_q | head -n1)"; then
    echo "failed searching /workspaces for repo $repo_basename" >&2
    exit 1
  fi
fi
if [[ -z "\$workspace_dir" ]]; then
  echo "unable to locate workspace root containing repo $repo_basename" >&2
  exit 1
fi
cd "\$workspace_dir"
export DECOMK_HOME=$decomk_home_q
export DECOMK_LOG_DIR=$decomk_log_dir_q
decomk run $decomk_run_action_args
EOF
)"

  codespace_bash "$run_script"
}

run_stage0_phase_with_stage0_env() {
  local stage0_phase="$1"
  local stage0_action_arg="$2"
  local github_user_value="$3"

  local stage0_phase_q
  local stage0_action_arg_q
  local github_user_value_q
  stage0_phase_q="$(printf '%q' "$stage0_phase")"
  stage0_action_arg_q="$(printf '%q' "$stage0_action_arg")"
  github_user_value_q="$(printf '%q' "$github_user_value")"

  local run_script
  run_script="$(cat <<EOF
set -euo pipefail
workspace_dir="/workspaces/$repo_basename"
if [[ ! -d "\$workspace_dir" ]]; then
  if ! workspace_dir="\$(find /workspaces -mindepth 1 -maxdepth 2 -type d -name $repo_basename_q | head -n1)"; then
    echo "failed searching /workspaces for repo $repo_basename" >&2
    exit 1
  fi
fi
if [[ -z "\$workspace_dir" ]]; then
  echo "unable to locate workspace root containing repo $repo_basename" >&2
  exit 1
fi
cd "\$workspace_dir"
export DECOMK_HOME=$decomk_home_q
export DECOMK_LOG_DIR=$decomk_log_dir_q
export DECOMK_TOOL_URI=$tool_uri_q
export GITHUB_USER=$github_user_value_q
bash examples/devcontainer/decomk-stage0.sh $stage0_phase_q $stage0_action_arg_q
EOF
)"

  codespace_bash "$run_script"
}

if ! run_logged run_decomk_with_stage0_env "TUPLE_STAMP_PROBE"; then
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

if ! run_logged run_decomk_with_stage0_env "TUPLE_STAMP_PROBE TUPLE_STAMP_VERIFY"; then
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

# Intent: Validate explicit stage-0 phase routing and GITHUB_USER handling by
# forcing one updateContent run (empty user) and one postCreate run
# (non-empty user), then asserting dedicated fixture markers.
# Source: DI-001-20260416-223600 (TODO/001)
if ! run_logged run_stage0_phase_with_stage0_env "updateContent" "TUPLE_PHASE_UPDATE" ""; then
  fail 33 "stage-0 updateContent phase run failed"
fi
phase_update_log_path="$(latest_make_log_path)"
if [[ -z "$phase_update_log_path" ]]; then
  fail 33 "could not find phase-update make.log"
fi
if [[ "$phase_update_log_path" == "$stamp_verify_log_path" ]]; then
  fail 33 "phase-update run reused stamp-verify make.log unexpectedly: $phase_update_log_path"
fi
load_make_log_or_fail 33 "$phase_update_log_path"
require_no_fail_markers_or_fail 33
require_marker_or_fail 33 "SELFTEST PASS phase-updateContent"
require_marker_or_fail 33 "SELFTEST PASS github-user-empty-in-updateContent"

runtime_github_user="$(codespace_bash 'printf %s "${GITHUB_USER:-}"')"
if [[ -z "$runtime_github_user" ]]; then
  fail 34 "codespace runtime GITHUB_USER is empty; cannot validate postCreate user-phase behavior"
fi
if ! run_logged run_stage0_phase_with_stage0_env "postCreate" "TUPLE_PHASE_POST" "$runtime_github_user"; then
  fail 34 "stage-0 postCreate phase run failed"
fi
phase_post_log_path="$(latest_make_log_path)"
if [[ -z "$phase_post_log_path" ]]; then
  fail 34 "could not find phase-post make.log"
fi
if [[ "$phase_post_log_path" == "$phase_update_log_path" ]]; then
  fail 34 "phase runs reused the same make.log path unexpectedly: $phase_post_log_path"
fi
load_make_log_or_fail 34 "$phase_post_log_path"
require_no_fail_markers_or_fail 34
require_marker_or_fail 34 "SELFTEST PASS phase-postCreate"
require_marker_or_fail 34 "SELFTEST PASS github-user-present-in-postCreate"

log "codespaces parity checks passed (including lifecycle phase checks)"
