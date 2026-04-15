# TODO 009 - phase-eval: empirical lifecycle spike for DevPod/Codespaces

## Decision Intent Log

ID: DI-009-20260413-232813
Date: 2026-04-13 23:28:13
Status: active
Decision: Add an experimental `examples/phase-eval` harness (plus `.devcontainer/phase-eval` config) that measures actual hook execution and `GITHUB_USER` availability across `devpod build`, `devpod up`, and Codespaces paths, and emits JSON + raw artifacts under `/tmp`.
Intent: Validate stage-0 lifecycle assumptions with direct evidence before finalizing decomk two-phase behavior and context-axis rules.
Constraints: Treat this as a POC spike (not a canonical decomk example); do not add `.github/workflows` in this phase; keep failures explicit with per-step rc/stdout/stderr artifacts.
Affects: `examples/phase-eval/*`, `.devcontainer/phase-eval/*`, `TODO/TODO.md`, `README.md`.

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

## Subtasks

- [x] 009.1 Add `examples/phase-eval/run.sh` orchestrator with explicit rc/stdout/stderr capture.
- [x] 009.2 Add `examples/phase-eval/hook_probe.sh` lifecycle probe script for hook markers + env snapshots.
- [x] 009.3 Add `examples/phase-eval/lib.sh` shared helpers and no-silent-failure utilities.
- [x] 009.4 Add `.devcontainer/phase-eval/devcontainer.json` and companion `Dockerfile` for evaluation runs.
- [x] 009.5 Add `examples/phase-eval/README.md` describing POC-spike status and usage.
- [ ] 009.6 Run phase-eval scenarios and record observed behavior summary in this TODO.
- [ ] 009.7 Use observed results to drive decomk lifecycle redesign decisions (selector mapping + context axes).
