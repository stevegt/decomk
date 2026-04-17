# TODO 009 - phase-eval: empirical lifecycle spike for DevPod/Codespaces

## Decision Intent Log

ID: DI-009-20260413-232813
Date: 2026-04-13 23:28:13
Status: active
Decision: Add an experimental `examples/phase-eval` harness (plus `.devcontainer/phase-eval` config) that measures actual hook execution and `GITHUB_USER` availability across `devpod build`, `devpod up`, and Codespaces paths, and emits JSON + raw artifacts under `/tmp`.
Intent: Validate stage-0 lifecycle assumptions with direct evidence before finalizing decomk two-phase behavior and context-axis rules.
Constraints: Treat this as a POC spike (not a canonical decomk example); do not add `.github/workflows` in this phase; keep failures explicit with per-step rc/stdout/stderr artifacts.
Affects: `examples/phase-eval/*`, `.devcontainer/phase-eval/*`, `TODO/TODO.md`, `README.md`.

ID: DI-009-20260415-081608
Date: 2026-04-15 08:16:08
Status: active
Decision: Sanitize generated DevPod workspace IDs in `examples/phase-eval/run.sh` so uppercase run-id fragments cannot violate DevPod naming constraints.
Intent: Keep phase-eval runs reproducible by preventing avoidable `devpod up` failures caused by invalid workspace IDs.
Constraints: Preserve readable UTC run IDs for artifact paths while guaranteeing workspace IDs contain only lowercase letters, digits, and dashes.
Affects: `examples/phase-eval/run.sh`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260415-081704
Date: 2026-04-15 08:17:04
Status: active
Decision: Make DevPod provider selection in phase-eval prefer read-only detection (`devpod provider list`) before attempting `provider use/add` writes.
Intent: Allow evaluation runs to proceed in environments where Docker provider is already configured but DevPod config writes are blocked.
Constraints: Keep explicit failure behavior when Docker provider is unavailable; do not silently skip provider setup failures.
Affects: `examples/phase-eval/run.sh`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260415-182045
Date: 2026-04-15 18:20:45
Status: active
Decision: Treat skipped or inconclusive phase-eval scenarios as failures by returning non-zero status for requested platforms, including Codespaces auth/prebuild-create gaps.
Intent: Prevent false-positive "success" runs when lifecycle evidence was not actually gathered, so conclusions remain evidence-based.
Constraints: Always write `summary.json` and artifacts before returning non-zero to preserve debugging context.
Affects: `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260415-182322
Date: 2026-04-15 18:23:22
Status: active
Decision: Add explicit Codespaces prebuild workflow trigger/wait/log capture in `examples/phase-eval/run.sh`, and record hook evidence from prebuild logs separately from codespace-create logs.
Intent: Empirically determine whether `updateContentCommand` executes during prebuilds instead of inferring from docs or startup-time hooks.
Constraints: Require a configured Codespaces prebuild workflow; preserve rc/log artifacts for every step; keep script behavior non-silent on inconclusive prebuild evaluation.
Affects: `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260415-182210
Date: 2026-04-15 18:22:10
Status: active
Decision: Add explicit Codespaces remote-devcontainer prechecks and capture prebuild-workflow discovery output via `run_capture` so evaluation failures are immediately diagnosable from artifacts.
Intent: Remove opaque harness failures where `codespace create` fails because the devcontainer path is missing on the remote branch or where prebuild workflow discovery fails silently.
Constraints: Keep non-zero failure semantics; avoid hidden command failures; preserve evidence in `raw/*` logs and `scenario-notes.tsv`.
Affects: `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260416-154702
Date: 2026-04-16 15:47:02
Status: active
Decision: Cross-link phase-eval findings to the deferred self-hosting runtime-adaptation decision tracked in TODO 007.14.
Intent: Ensure lifecycle evidence from this spike directly informs later decisions about hook-inclusive self-hosted prebuild behavior.
Constraints: Keep TODO 009 focused on evidence collection; keep runtime-adaptation implementation work deferred to TODO 007.
Affects: `TODO/009-phase-eval-lifecycle-spike.md`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-009-20260417-030759
Date: 2026-04-17 03:07:59
Status: active
Decision: Add phase-persistent hook artifacts and strict Codespaces sequencing checks in the phase-eval harness so we can prove prebuild hook execution separately from first-boot hook execution.
Intent: Ensure evaluation conclusions come from durable in-container evidence tied to specific phases (prebuild vs runtime), with explicit push/prebuild/create ordering checks.
Constraints: Keep no-silent-failure behavior, preserve machine-readable summary output, and avoid relying on docs-only assumptions about hook timing.
Affects: `examples/phase-eval/hook_probe.sh`, `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

## Goal

Produce reproducible evidence for lifecycle behavior so design decisions about
`updateContentCommand`, `postCreateCommand`, and user-context handling do not
rely on docs-only assumptions.

## Scope

In scope:
- Add runnable evaluator scripts under `examples/phase-eval/`.
- Add a dedicated phase-eval devcontainer config under `.devcontainer/phase-eval/`.
- Capture run artifacts under `/tmp/decomk-phase-eval.<runid>/`.

Out of scope:
- Changing decomk core lifecycle behavior in this TODO.
- Adding permanent GitHub workflow automation.
- Implementing runtime-adaptation/fork work for hook-inclusive self-hosted prebuilds (tracked as `TODO/007.14`).

## Subtasks

- [x] 009.1 Add `examples/phase-eval/run.sh` orchestrator with explicit rc/stdout/stderr capture.
- [x] 009.2 Add `examples/phase-eval/hook_probe.sh` lifecycle probe script for hook markers + env snapshots.
- [x] 009.3 Add `examples/phase-eval/lib.sh` shared helpers and no-silent-failure utilities.
- [x] 009.4 Add `.devcontainer/phase-eval/devcontainer.json` and companion `Dockerfile` for evaluation runs.
- [x] 009.5 Add `examples/phase-eval/README.md` describing POC-spike status and usage.
- [x] 009.8 Add explicit Codespaces prebuild workflow trigger/wait/log capture path.
- [x] 009.9 Link deferred self-hosting runtime-adaptation follow-up to `TODO/007.14`.
- [x] 009.10 Add durable phase markers to distinguish prebuild hook execution from first-boot hook execution.
- [ ] 009.6 Run phase-eval scenarios and record observed behavior summary in this TODO.
- [ ] 009.7 Use observed results to drive decomk lifecycle redesign decisions (selector mapping + context axes).
