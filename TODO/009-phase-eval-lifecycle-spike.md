# TODO 009 - phase-eval: empirical lifecycle spike for devcontainer/DevPod/Codespaces

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

ID: DI-009-20260417-031847
Date: 2026-04-17 03:18:47
Status: active
Decision: Replace explicit `gh workflow run` dispatch for Codespaces prebuilds with polling/watching the push-triggered prebuild workflow run for `origin/<branch>` head SHA, and fix remote devcontainer existence checks to use GET semantics.
Intent: Prevent false early failures when the built-in Codespaces prebuild workflow is not dispatchable via `workflow_dispatch`, while preserving strict push→prebuild→codespace sequencing guarantees.
Constraints: Keep explicit non-zero failures when matching prebuild run cannot be found/completed; keep artifact evidence and scenario notes actionable.
Affects: `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260416-222400
Date: 2026-04-16 22:24:00
Status: active
Decision: Close the lifecycle spike with an explicit contract: `updateContentCommand` is the prebuild/common phase, `postCreateCommand` is the runtime/user phase, and `GITHUB_USER` is treated as runtime-only user identity data.
Intent: Lock design decisions to empirical evidence so stage-0 hook behavior is not inferred from docs-only assumptions.
Constraints: Base conclusions on durable hook artifacts and run summary fields; keep evidence paths in this TODO for auditability.
Affects: `TODO/009-phase-eval-lifecycle-spike.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`, `TODO/006-decomk-user-customization-via-github-user.md`.

ID: DI-009-20260418-130656
Date: 2026-04-18 13:06:56
Status: active
Decision: Add devcontainer CLI as a first-class phase-eval platform with two evidence-checked runs (`devcontainer up --prebuild` and `devcontainer up`), and rename the aggregate selector from `both` to `all` (no compatibility alias).
Intent: Validate lifecycle hook behavior under the reference devcontainer runtime in addition to DevPod/Codespaces so platform conclusions are grounded in direct evidence.
Constraints: Keep no-silent-failure behavior; gate `devcontainer` success on hook evidence (prebuild must show `updateContent` without `postCreate`, runtime must show `postCreate`); keep summary output machine-readable.
Affects: `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.
Supersedes: DI-009-20260413-232813

ID: DI-009-20260418-135200
Date: 2026-04-18 13:52:00
Status: active
Decision: Add `onCreate` lifecycle evidence capture to phase-eval across devcontainer, DevPod, and Codespaces by wiring `onCreateCommand` in the eval devcontainer config and emitting per-scenario `onCreate_seen` fields in `summary.json`.
Intent: Expand lifecycle observability to all standard devcontainer hooks while keeping current stability by reporting `onCreate` as informational evidence instead of a pass/fail gate.
Constraints: Preserve existing failure gates; avoid introducing ordering assertions until platform behavior is validated over more runs.
Affects: `.devcontainer/phase-eval/devcontainer.json`, `examples/phase-eval/run.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260418-140800
Date: 2026-04-18 14:08:00
Status: active
Decision: Fix phase-eval hook extraction to capture embedded `PHASE_EVAL_EVENT|...` payloads from prefixed logs, update Codespaces phase bucketing so `onCreate`+`updateContent` remain classified as prebuild before the prebuild marker exists, and emit `user_nonempty` alongside `github_user_nonempty` for each scenario.
Intent: Eliminate false negatives in Codespaces prebuild evidence after enabling `onCreateCommand`, and explicitly surface whether `USER` is populated during each evaluated lifecycle path.
Constraints: Preserve existing pass/fail gates and keep `onCreate` evidence informational; avoid silent parsing failures.
Affects: `examples/phase-eval/run.sh`, `examples/phase-eval/hook_probe.sh`, `examples/phase-eval/README.md`, `TODO/009-phase-eval-lifecycle-spike.md`.

ID: DI-009-20260419-100900
Date: 2026-04-19 10:09:00
Status: active
Decision: Promote `/tmp/decomk-phase-eval.20260419T014758Z-3468588/summary.json` as canonical lifecycle evidence and update TODO 009 conclusions to include `onCreate` coverage plus dual identity signals (`GITHUB_USER`, `USER`) per phase/platform.
Intent: Keep lifecycle and identity contracts anchored to the latest full-platform empirical run, including the post-`onCreate` extraction/bucketing fixes.
Constraints: Keep evidence statements traceable to concrete artifact paths; avoid introducing new runtime behavior changes in this TODO-only update.
Affects: `TODO/009-phase-eval-lifecycle-spike.md`.
Supersedes: DI-009-20260416-222400

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
- [x] 009.11 Add devcontainer CLI platform evaluation path (`--platform devcontainer`) and rename aggregate selector to `all`.
- [x] 009.12 Add `onCreate` hook evidence checks for all phase-eval platforms and expose `onCreate_seen` in run summary output.
- [x] 009.13 Fix prefixed log extraction and Codespaces phase bucketing for `onCreate` + add `user_nonempty` evidence fields.
- [x] 009.14 Promote `20260419T014758Z-3468588` as canonical evidence and refresh TODO 009 lifecycle/identity conclusions.
- [x] 009.6 Run phase-eval scenarios and record observed behavior summary in this TODO.
- [x] 009.7 Use observed results to drive decomk lifecycle redesign decisions (selector mapping + context axes).

## Observed behavior summary

Evidence run:
- `summary.json`: `/tmp/decomk-phase-eval.20260419T014758Z-3468588/summary.json`
- `scenario-notes.tsv`: `/tmp/decomk-phase-eval.20260419T014758Z-3468588/scenario-notes.tsv`
- persistent hook events: `/tmp/decomk-phase-eval.20260419T014758Z-3468588/codespaces-persistent.events.log`

Observed facts:
- devcontainer prebuild (`devcontainer up --prebuild`) emitted `onCreate` + `updateContent`, and did not emit `postCreate`.
- devcontainer runtime (`devcontainer up`) emitted `onCreate` + `updateContent` + `postCreate`.
- DevPod build emitted no lifecycle hook events; DevPod runtime (`devpod up`) emitted all three hooks (`onCreate`, `updateContent`, `postCreate`).
- Codespaces prebuild run emitted `onCreate` + `updateContent` and did not emit `postCreate`.
- Codespaces runtime/create emitted `onCreate` + `updateContent` + `postCreate`.
- Codespaces persistent evidence confirms prebuild/runtime split:
  - `persistent_prebuild_update_event_seen=true`
  - `persistent_runtime_postcreate_event_seen=true`
  - `persistent_prebuild_update_marker_present=true`
  - `persistent_runtime_postcreate_marker_present=true`
- Identity signal cross-check:
  - Codespaces prebuild: `github_user_nonempty=false`, `user_nonempty=false`
  - Codespaces runtime/create: `github_user_nonempty=true`, `user_nonempty=false`
  - DevPod runtime: `github_user_nonempty=false`, `user_nonempty=true`

Design conclusion:
- `onCreateCommand` + `updateContentCommand` are prebuild/common lifecycle signals.
- `postCreateCommand` is the runtime/user phase signal.
- Runtime identity should resolve as `GITHUB_USER` first, with `USER` as a fallback when `GITHUB_USER` is unset.
- In Codespaces specifically, `USER` may remain empty even at runtime; `GITHUB_USER` is the reliable user-phase identity signal.
