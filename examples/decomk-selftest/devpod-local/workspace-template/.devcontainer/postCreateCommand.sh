#!/usr/bin/env bash

set -euo pipefail

# Intent: Implement generic stage-0 bootstrap with install-first tool delivery:
# install decomk via go install by default, optionally clone+pull then install in
# repo mode, and keep config repo sync separate.
# Source: DI-001-20260311-163942 (TODO/001)
DECOMK_HOME="${DECOMK_HOME:-/var/decomk}"
DECOMK_LOG_DIR="${DECOMK_LOG_DIR:-/var/log/decomk}"
DECOMK_TOOL_MODE="${DECOMK_TOOL_MODE:-install}"
DECOMK_TOOL_INSTALL_PKG="${DECOMK_TOOL_INSTALL_PKG:-github.com/stevegt/decomk/cmd/decomk@latest}"
DECOMK_TOOL_REPO="${DECOMK_TOOL_REPO:-https://github.com/stevegt/decomk}"

export DECOMK_HOME DECOMK_LOG_DIR DECOMK_TOOL_MODE DECOMK_TOOL_INSTALL_PKG DECOMK_TOOL_REPO

sync_git_repo() {
  local repo_url="$1"
  local repo_dir="$2"

  if [[ -z "$repo_url" ]]; then
    return 0
  fi

  if [[ -d "$repo_dir/.git" ]]; then
    git -C "$repo_dir" remote set-url origin "$repo_url"
    git -C "$repo_dir" pull --ff-only
    return 0
  fi

  if [[ -e "$repo_dir" ]]; then
    echo "decomk bootstrap: path exists but is not a git repo: $repo_dir" >&2
    return 1
  fi

  mkdir -p "$(dirname "$repo_dir")"
  git clone "$repo_url" "$repo_dir"
}

resolve_go_bin_dir() {
  local gobin
  gobin="$(go env GOBIN)"
  if [[ -n "$gobin" ]]; then
    printf '%s' "$gobin"
    return 0
  fi

  local gopath
  local gopath_first
  gopath="$(go env GOPATH)"
  gopath_first="${gopath%%:*}"
  if [[ -z "$gopath_first" ]]; then
    return 1
  fi
  printf '%s/bin' "$gopath_first"
}

install_decomk() {
  case "$DECOMK_TOOL_MODE" in
    install)
      go install "$DECOMK_TOOL_INSTALL_PKG"
      ;;
    clone)
      local tool_src_dir="$DECOMK_HOME/src/decomk"
      sync_git_repo "$DECOMK_TOOL_REPO" "$tool_src_dir"
      (cd "$tool_src_dir" && go install ./cmd/decomk)
      ;;
    *)
      echo "decomk bootstrap: invalid DECOMK_TOOL_MODE=$DECOMK_TOOL_MODE (expected install or clone)" >&2
      return 1
      ;;
  esac
}

resolve_decomk_binary() {
  local go_bin_dir
  go_bin_dir="$(resolve_go_bin_dir)"
  if [[ -n "$go_bin_dir" ]] && [[ -x "$go_bin_dir/decomk" ]]; then
    printf '%s' "$go_bin_dir/decomk"
    return 0
  fi
  if command -v decomk >/dev/null 2>&1; then
    command -v decomk
    return 0
  fi
  return 1
}

mkdir -p "$DECOMK_HOME" "$DECOMK_LOG_DIR"

install_decomk
sync_git_repo "${DECOMK_CONF_REPO:-}" "$DECOMK_HOME/conf"

if [[ -z "${DECOMK_CONF_REPO:-}" ]] && [[ ! -f "$DECOMK_HOME/conf/decomk.conf" ]]; then
  echo "decomk bootstrap: no DECOMK_CONF_REPO and no $DECOMK_HOME/conf/decomk.conf; skipping decomk run" >&2
  exit 1
fi

decomk_bin="$(resolve_decomk_binary)" || {
  echo "decomk bootstrap: could not find installed decomk binary (tried go install target and PATH)" >&2
  exit 1
}

# shellcheck disable=SC2086
"$decomk_bin" run ${DECOMK_RUN_ARGS:-all}
