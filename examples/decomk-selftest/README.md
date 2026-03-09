# decomk self-test

This directory contains reproducible integration checks for `decomk` runtime behavior.

## Current scope

The current harness scope is:

- Local DevPod with Docker provider (automated)

The next planned scope is:

- Codespaces parity checks (after local DevPod scenarios stay green)

Deferred scope (decision pending):

- DevPod remote GCP provider checks

## Why this exists

The self-test validates the reference bootstrap flow in `examples/devcontainer/postCreateCommand.sh` under controlled scenarios so changes in privilege handling, env propagation, and path selection are caught before rollout.

## Prerequisites

- `devpod` CLI installed and on `PATH`
- Local Docker engine running
- `git` available locally

The harness configures/uses the DevPod Docker provider and runs each scenario in an isolated temporary workspace clone.

## Run

Run all scenarios:

```bash
examples/decomk-selftest/devpod-local/run.sh --scenario all
```

Run one scenario:

```bash
examples/decomk-selftest/devpod-local/run.sh --scenario no_sudo_make_as_user
```

## Automated scenarios

- `root_hook_owner_inferred`: execute bootstrap as root while exporting `GITHUB_USER=dev` so owner inference path is exercised.
- `non_root_default_make_as_root`: execute bootstrap as `dev` with default root-make behavior.
- `no_sudo_expect_fail`: execute as `dev` with a fake `sudo` shim first in `PATH`; expects clear failure.
- `no_sudo_make_as_user`: same fake `sudo` setup with `DECOMK_MAKE_AS_ROOT=false`; expects success.

## Assertions

Passing scenarios require:

- `<DECOMK_HOME>/env.sh` exists
- expected stamp target exists under `<DECOMK_HOME>/stamps/`
- at least one non-empty `make.log` exists under `<DECOMK_LOG_DIR>/<runID>/`
- expected `DECOMK_MAKE_USER` and `DECOMK_DEV_USER` values appear in `env.sh`

The expected-failure scenario additionally validates that the failure includes `need passwordless sudo`.

## Output location in workspace

Each scenario writes a summary file inside the workspace container:

- `/tmp/decomk-selftest-results/<scenario>/result.env`

`run.sh` reads that file via DevPod SSH and treats anything except `OUTCOME=PASS` as failure.
