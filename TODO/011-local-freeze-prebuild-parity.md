# TODO 011 - local freeze pipeline for prebuild parity

## Decision Intent Log

ID: DI-011-20260417-135200
Date: 2026-04-17 13:52:00
Status: active
Decision: Adopt Option 2 (controlled local/CI freeze pipeline) as the active image-management path for decomk blocks, and explicitly track parity verification against Codespaces prebuild behavior.
Intent: Ensure frozen images include both Dockerfile and decomk `updateContent` effects while using a path we can operate today.
Constraints: Treat direct prebuild export/promotion as unavailable for now; keep lifecycle split (`updateContent` common/prebuild, `postCreate` runtime/user) intact; keep parity-proof model decision deferred to a dedicated bakeoff step.
Affects: `doc/image-management.md`, `TODO/011-local-freeze-prebuild-parity.md`, `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/007-devpod-gcp-selfhost-migration.md`, `TODO/TODO.md`.

## Goal

Implement a deterministic local/CI freeze process that produces
decomk-managed block images with measurable parity against Codespaces
prebuild behavior.

## Background

See `doc/image-management.md` for rationale, option analysis, and
design constraints.

## Scope

In scope:

- Define parity artifacts and comparison contract.
- Implement a freeze runner that executes Dockerfile + decomk
  `updateContent` phase.
- Add parity checks and acceptance gates.
- Document operator workflow for freeze/verify/promote.

Out of scope:

- Assuming availability of direct Codespaces prebuild export/promotion.
- Folding runtime/user customization (`postCreate`) into frozen shared
  block images.
- GCP rollout execution details (tracked in TODO 007).

## Dependencies and links

- Lifecycle evidence baseline: `TODO/009-phase-eval-lifecycle-spike.md`
- Block profile selection model: `TODO/010-codespaces-block-prebuild-profiles.md`
- Self-host migration track: `TODO/007-devpod-gcp-selfhost-migration.md`

## Subtasks

- [ ] 011.1 Define canonical parity artifact schema (image metadata, manifests, lifecycle markers, and provenance fields).
- [ ] 011.2 Run a parity proof-model bakeoff and lock the acceptance model for this TODO.
- [ ] 011.3 Implement the local/CI freeze runner that executes Dockerfile build followed by `.devcontainer/decomk-stage0.sh updateContent`.
- [ ] 011.4 Enforce phase separation so frozen images exclude runtime/user-phase (`postCreate`) side effects.
- [ ] 011.5 Add deterministic pinning checks (base digest, package/tool versions, git refs, and other mutable inputs).
- [ ] 011.6 Implement parity comparator tooling and machine-readable failure reports.
- [ ] 011.7 Add repeatable operator/CI entrypoints and documentation for freeze + parity verification.
- [ ] 011.8 Execute acceptance matrix runs and record evidence paths/results in this TODO.

## Acceptance criteria

- Repeated freezes with identical inputs produce stable outputs under the
  locked parity model.
- Parity checks against Codespaces baseline pass under the locked model.
- Drift is classified explicitly; unapproved drift fails the gate.
- Freeze workflow is documented and runnable without hidden/manual steps.
