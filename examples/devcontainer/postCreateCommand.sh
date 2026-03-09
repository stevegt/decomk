#!/usr/bin/env bash
#
# Example devcontainer `postCreateCommand` bootstrap for decomk.
#
# This script is intended to be copied into other repos' `.devcontainer/`
# directories. It:
#   1) ensures decomk state/log directories exist and are writable
#   2) clones/updates decomk under $DECOMK_HOME/decomk
#   3) runs `decomk run`
#
# Configuration:
#   - DECOMK_HOME      (default /var/decomk) controls state (conf clone, stamps, env.sh)
#   - DECOMK_LOG_DIR   (default /var/log/decomk) controls per-run logs
#   - DECOMK_MAKE_AS_ROOT (default true) controls whether decomk runs make as root (via passwordless sudo -n when decomk is non-root)
#   - DECOMK_CONF_REPO (optional) URL for the shared config repo
#   - DECOMK_TOOL_REPO (optional) URL for the decomk tool repo (defaults upstream)
#
# Note: decomk itself can also self-update, but this script keeps the initial
# acquisition step simple and predictable.
set -euo pipefail

DECOMK_HOME="${DECOMK_HOME:-/var/decomk}"
DECOMK_LOG_DIR="${DECOMK_LOG_DIR:-/var/log/decomk}"
DECOMK_TOOL_REPO="${DECOMK_TOOL_REPO:-https://github.com/stevegt/decomk}"
export DECOMK_HOME DECOMK_LOG_DIR DECOMK_TOOL_REPO

# Export additional DECOMK_* knobs so that when this script runs as root and
# re-invokes decomk as the dev user (via runuser/sudo + env whitelist), decomk
# sees the same configuration the script used for validation.
export DECOMK_CONF_REPO DECOMK_MAKE_AS_ROOT DECOMK_CONFIG DECOMK_CONTEXT DECOMK_WORKSPACES_DIR

echo "decomk bootstrap:"
echo "  DECOMK_HOME=$DECOMK_HOME"
echo "  DECOMK_LOG_DIR=$DECOMK_LOG_DIR"
echo "  DECOMK_MAKE_AS_ROOT=${DECOMK_MAKE_AS_ROOT:-}"
echo "  DECOMK_TOOL_REPO=$DECOMK_TOOL_REPO"
echo "  DECOMK_CONF_REPO=${DECOMK_CONF_REPO:-}"

resolve_owner_user() {
  # Best effort: determine the intended non-root dev user for the container.
  #
  # If the script is running as the dev user already, just return the current
  # user. If the script is running as root (some devcontainer hosts do this),
  # try to infer the non-root user from environment variables or from workspace
  # directory ownership.
  #
  # This intentionally avoids guessing common usernames (like "vscode") because
  # picking the wrong user is worse than falling back to running as root with a
  # clear warning.
  # Intent: Avoid misidentifying the dev user by using only reliable inference
  # signals; when inference fails, warn and let callers choose a safe fallback.
  # Source: DI-001-20260309-172358 (TODO/001)
  if [[ "$(id -u)" -ne 0 ]]; then
    id -un
    return 0
  fi

  local u=""
  for u in "${SUDO_USER:-}" "${REMOTE_USER:-}" "${USERNAME:-}" "${GITHUB_USER:-}" "${USER:-}"; do
    if [[ -z "$u" ]]; then
      continue
    fi
    if ! id -u "$u" >/dev/null 2>&1; then
      continue
    fi
    if [[ "$(id -u "$u")" -eq 0 ]]; then
      continue
    fi
    echo "$u"
    return 0
  done

  # If postCreateCommand is running as root, the current working directory is
  # often the workspace checkout, which is typically owned by the dev user.
  # Use that ownership as a hint.
  local pwd_uid=""
  pwd_uid="$(stat -c '%u' . 2>/dev/null || true)"
  if [[ -n "$pwd_uid" ]] && [[ "$pwd_uid" -ne 0 ]]; then
    local pwd_user=""
    pwd_user="$(awk -F: -v uid="$pwd_uid" '$3==uid {print $1; exit}' /etc/passwd 2>/dev/null || true)"
    if [[ -n "$pwd_user" ]] && id -u "$pwd_user" >/dev/null 2>&1; then
      if [[ "$(id -u "$pwd_user")" -ne 0 ]]; then
        echo "$pwd_user"
        return 0
      fi
    fi
  fi

  # Unknown: callers can decide whether to skip chown, drop privileges, or fail.
  echo ""
}

resolve_owner() {
  # Best effort: try to determine the intended "dev user" that should own
  # decomk's state/log directories.
  #
  # In many environments, postCreateCommand runs as the dev user and we can use
  # our own uid/gid. In others it may run as root; in that case we try to infer
  # the non-root user from common devcontainer environment variables.
  if [[ "$(id -u)" -ne 0 ]]; then
    echo "$(id -u):$(id -g)"
    return 0
  fi

  local u=""
  u="$(resolve_owner_user)"
  if [[ -n "$u" ]]; then
    echo "$(id -u "$u"):$(id -g "$u")"
    return 0
  fi

  # Unknown; callers can decide whether to skip chown or fail.
  echo ""
}

require_passwordless_sudo() {
  if [[ "$(id -u)" -eq 0 ]]; then
    return 0
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    echo "decomk bootstrap: need passwordless sudo (sudo not found)" >&2
    exit 1
  fi
  if ! sudo -n true 2>/dev/null; then
    echo "decomk bootstrap: need passwordless sudo (sudo -n failed); configure your image/user or set DECOMK_MAKE_AS_ROOT=false and use user-writable DECOMK_HOME/DECOMK_LOG_DIR" >&2
    exit 1
  fi
}

run_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  else
    require_passwordless_sudo
    sudo -n -- "$@"
  fi
}

run_as_owner() {
  # Run a command as the intended dev user.
  #
  # If the script is running as non-root, this is a no-op wrapper.
  # If the script is running as root, use runuser/sudo to drop privileges so we
  # don't leave root-owned decomk state behind.
  if [[ "$(id -u)" -ne 0 ]]; then
    "$@"
    return
  fi

  local u=""
  u="$(resolve_owner_user)"
  if [[ -z "$u" ]]; then
    # Intent: If the dev user cannot be determined, do not guess (which can
    # produce confusing ownership and git safety failures). Proceed as root with
    # clear warnings so the container create step remains debuggable.
    # Source: DI-001-20260309-172358 (TODO/001)
    echo "decomk bootstrap: warning: could not determine non-root dev user (checked env vars and workspace ownership); running as root: $*" >&2
    echo "decomk bootstrap: warning: to avoid root-owned state, ensure the workspace is owned by the dev user and/or set REMOTE_USER/USER appropriately" >&2
    "$@"
    return
  fi

  local wl=""
  wl="DECOMK_HOME,DECOMK_LOG_DIR,DECOMK_MAKE_AS_ROOT,DECOMK_CONF_REPO,DECOMK_TOOL_REPO,DECOMK_CONFIG,DECOMK_CONTEXT,DECOMK_WORKSPACES_DIR,PATH"

  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$u" -w "$wl" -- "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo -u "$u" --preserve-env="$wl" -- "$@"
    return
  fi

  echo "decomk bootstrap: warning: neither runuser nor sudo found; running as root: $*" >&2
  "$@"
}

ensure_dir_owned() {
  local path="$1"
  if mkdir -p "$path" 2>/dev/null; then
    :
  else
    run_root mkdir -p "$path"
  fi

  local owner
  owner="$(resolve_owner)"

  if [[ -n "$owner" ]]; then
    # If we can't write, or if the directory root isn't owned by the dev user,
    # fix ownership with sudo (or as root).
    local current=""
    current="$(stat -c '%u:%g' "$path" 2>/dev/null || true)"
    if [[ ! -w "$path" ]] || [[ "$current" != "$owner" ]]; then
      run_root chown -R "$owner" "$path"
    fi
  else
    echo "decomk bootstrap: could not determine non-root owner; leaving $path ownership unchanged" >&2
  fi
}

ensure_dir_owned "$DECOMK_HOME"
ensure_dir_owned "$DECOMK_LOG_DIR"

# decomk runs as the dev user, but by default it runs make via passwordless sudo
# (-make-as-root=true). Fail fast with a clear message if sudo isn't available.
case "$(echo "${DECOMK_MAKE_AS_ROOT:-true}" | tr '[:upper:]' '[:lower:]' | xargs)" in
0|false|f)
  # make is not expected to run as root, so sudo isn't required.
  ;;
*)
  if [[ "$(id -u)" -ne 0 ]]; then
    require_passwordless_sudo
  else
    # If postCreateCommand runs as root but we run decomk as the dev user, that
    # user must still have passwordless sudo for decomk's default "make as root"
    # behavior.
    u="$(resolve_owner_user)"
    if [[ -n "$u" ]]; then
      if ! run_as_owner sudo -n true 2>/dev/null; then
        echo "decomk bootstrap: need passwordless sudo for dev user $u (sudo -n failed); configure your image/user or set DECOMK_MAKE_AS_ROOT=false and use user-writable DECOMK_HOME/DECOMK_LOG_DIR" >&2
        exit 1
      fi
    fi
  fi
  ;;
esac

tool_dir="$DECOMK_HOME/decomk"
if [[ -d "$tool_dir/.git" ]]; then
  echo "decomk bootstrap: updating tool repo in $tool_dir"
  run_as_owner git -C "$tool_dir" pull --ff-only || true
elif [[ -e "$tool_dir" ]]; then
  echo "decomk bootstrap: $tool_dir exists but is not a git repo; skipping clone" >&2
else
  echo "decomk bootstrap: cloning tool repo into $tool_dir"
  run_as_owner git clone "$DECOMK_TOOL_REPO" "$tool_dir"
fi

# If we have neither an explicit config repo URL nor an existing config file, do
# not fail the container create step; print a hint and exit successfully.
if [[ -z "${DECOMK_CONF_REPO:-}" ]] && [[ ! -f "$DECOMK_HOME/conf/decomk.conf" ]]; then
  echo "decomk bootstrap: no DECOMK_CONF_REPO and no $DECOMK_HOME/conf/decomk.conf; skipping decomk run" >&2
  echo "decomk bootstrap: set DECOMK_CONF_REPO or use -config/-makefile in your devcontainer" >&2
  exit 0
fi

echo "decomk bootstrap: running decomk"
run_as_owner bash -lc "cd $(printf %q "$tool_dir") && go run ./cmd/decomk run"

owner="$(resolve_owner)"
if [[ "$(id -u)" -eq 0 ]] && [[ -n "$owner" ]]; then
  # Best-effort cleanup for hosts that run postCreateCommand as root.
  #
  # decomk should have run as the dev user via run_as_owner above, but if that
  # failed for any reason, make sure we don't leave a root-owned tree behind.
  chown -R "$owner" "$DECOMK_HOME" "$DECOMK_LOG_DIR" || true
fi
