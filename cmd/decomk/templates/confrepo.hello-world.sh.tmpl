#!/usr/bin/env bash

set -euo pipefail

# This script is intentionally tiny.
# It demonstrates how Makefile targets can call scripts from the shared conf
# repo and make changes to the devcontainer filesystem.

# tee outputs to both stdout and a log file, so we can see the output in the terminal
exec > /tmp/hello-world.log 2>&1

target_name="${1:-}"
hello_text="${2:-Hello from bin/hello-world.sh}"

if [[ -z "$target_name" ]]; then
  echo "usage: hello-world.sh <target-name> [message]" >&2
  exit 2
fi

echo "HELLO target=${target_name}"
echo "HELLO message=${hello_text}"
echo "HELLO phase=${DECOMK_STAGE0_PHASE:-<unset>}"
echo "HELLO gui=${DEVCONTAINER_GUI:-<unset>}"
echo "HELLO github_user=${GITHUB_USER:-<unset>}"
