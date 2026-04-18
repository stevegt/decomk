# phase-eval (POC spike)

`examples/phase-eval` is an empirical spike to measure lifecycle behavior across
devcontainer CLI, DevPod, and Codespaces.

## Important scope note

This is **not** a canonical decomk usage example.
It is a **POC spike** used to validate assumptions before changing decomk
stage-0 lifecycle design.

## What it evaluates

The spike captures evidence for:

- whether `onCreateCommand` runs,
- whether `updateContentCommand` runs,
- whether `postCreateCommand` runs,
- whether `GITHUB_USER` is populated,

across:

- `devcontainer up --prebuild`,
- `devcontainer up`,
- `devpod build`,
- `devpod up`,
- Codespaces prebuild-list/API visibility,
- Codespaces prebuild workflow trigger/wait/log capture,
- Codespaces create/start behavior.

## Files

- `run.sh` — orchestrates scenarios and writes artifacts.
- `hook_probe.sh` — called by lifecycle hooks; emits marker lines and env snapshots.
- `lib.sh` — shared logging + command capture helpers.
- `.devcontainer/phase-eval/devcontainer.json` — evaluation-specific lifecycle config.

## Artifacts

By default, output is written under:

- `/tmp/decomk-phase-eval.<runid>/`

Key files:

- `summary.json`
- `summary.json` scenario fields now include `onCreate_seen` (informational)
- `raw/*.stdout.log`, `raw/*.stderr.log`, `raw/*.rc`
- `devcontainer-prebuild.events.log`
- `devcontainer-up.events.log`
- `codespaces-prebuild.events.log` (when Codespaces prebuild logs contain hook markers)
- `codespaces-persistent.events.log` (durable in-container event history fetched after codespace start)
- `scenario-notes.tsv`
- `diagnostics.complete`

## Usage

From repository root:

```bash
examples/phase-eval/run.sh --platform all
```

devcontainer CLI only:

```bash
examples/phase-eval/run.sh --platform devcontainer
```

DevPod only:

```bash
examples/phase-eval/run.sh --platform devpod
```

Codespaces only:

```bash
examples/phase-eval/run.sh --platform codespaces
```

Keep remote workspace/codespace when failing:

```bash
examples/phase-eval/run.sh --keep-on-fail
```

## Prerequisites

- DevPod + Docker for DevPod scenarios.
- devcontainer CLI (`devcontainer`) + Docker for local devcontainer scenarios.
- `gh` auth for Codespaces scenarios.
- Repository branch pushed for Codespaces create checks.
- Codespaces prebuilds configured for the repo (the harness triggers the prebuild workflow and waits for completion).
- `.devcontainer/phase-eval/devcontainer.json` committed on the evaluated branch (Codespaces resolves this path from remote repo contents).

## Codespaces sequencing used by the harness

For `--platform codespaces`, the harness enforces this order:

1. Verify local `HEAD` equals `origin/<branch>` (prevents stale unpushed evaluation).
2. Find the push-triggered Codespaces prebuild workflow run for `origin/<branch>` HEAD and wait for completion.
3. Create/start a Codespace from the same branch.
4. Fetch durable hook artifacts from `$HOME/.decomk-phase-eval-hooks` inside the Codespace.

The run fails if durable evidence does not show:

- `updateContent` recorded in `phase_bucket=prebuild`
- `postCreate` recorded in `phase_bucket=runtime`

For `--platform devcontainer`, the harness enforces:

1. A prebuild run (`devcontainer up --prebuild`) where hook evidence must include `updateContent` and must not include `postCreate`.
2. A runtime run (`devcontainer up`) where hook evidence must include `postCreate`.

For all platforms, the harness records `onCreate_seen` in `summary.json` for
each scenario. `onCreate_seen` is currently evidence-only (informational) and
is not yet part of pass/fail gating.

## Interpretation

This spike intentionally favors explicit evidence over assumptions. If a platform
API is unavailable, the run records that condition in artifacts and exits
non-zero for the requested platform evaluation.
