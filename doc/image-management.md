# Image Management for Decomk: Single-Path Checkpoints

## Purpose

This document records why decomk uses single-path checkpoints for shared
block images and how that design reduces prebuild and first-boot time.

This is a background/design document. Implementation work is tracked in
`TODO/011-single-path-checkpoints.md`.

## Primary objective

The objective is speed: reduce repeated setup work during prebuild and
first boot by freezing shared setup into checkpoint images at block
boundaries.

No separate performance-metric framework is defined here. The design
goal is architectural: shift repeated work into earlier checkpoint
layers so later runs do less setup.

## Why Dockerfile-only freeze is insufficient

For decomk-managed environments, effective shared state comes from both:

1. Dockerfile layer execution, and
2. decomk flow:
   `updateContent` → `decomk-stage0.sh updateContent` → `decomk run` →
   config-repo Makefile effects.

A Dockerfile-only freeze misses decomk-managed setup and therefore
cannot be the canonical block-freeze path.

## Single-path checkpoint design

The key design rule is image equivalence by path construction:

- Prebuild path and checkpoint-build path both execute the same
  `updateContent -> decomk run` flow.
- User/runtime path (`postCreate`) is intentionally excluded from frozen
  shared block images.

This gives reasonable behavioral equivalence without requiring a
separate comparator-heavy parity model as a primary requirement.

## Lifecycle facts this design depends on

Empirical results in `TODO/009-phase-eval-lifecycle-spike.md` show:

- `updateContentCommand` is prebuild/common phase.
- `postCreateCommand` is runtime/user phase.
- `GITHUB_USER` is runtime/user-phase data.

Therefore, shared checkpoint images are built from `updateContent` only.

## Options considered

### Option 1: Promote GitHub prebuild artifact directly

**Idea:** export/promote the opaque prebuild snapshot directly.

**Result:** impossible with current supported github API and UI interfaces.

### Option 2: Local/CI single-path checkpoint pipeline (selected)

**Idea:** run Dockerfile + `updateContent` in a controlled flow, then
commit the resulting container as the next block checkpoint image.

**Result:** selected as the viable path available now.

### Option 3: Dockerfile-only freeze

**Idea:** freeze only Dockerfile layers.

**Result:** rejected; incomplete for decomk-managed shared setup.

## Operational model

1. Run `decomk checkpoint build` to create a local candidate image by
   executing prebuild/common flow through `updateContent`.
2. Run `decomk checkpoint push <source> <immutable> <testing|unstable>`
   to publish candidate tags for external/manual testing.
3. After tests pass, run `decomk checkpoint tag -m <source> stable` to
   move stable explicitly.
4. Configure consumer repos to reference channel tags (auto-follow) or
   pinned tags (intentional hold) in `.devcontainer/devcontainer.json`.
5. Keep runtime/user setup in `postCreate`, outside shared checkpoints.

Notes:

- "Immutable tag" and "channel tag" are both plain registry tags; the
  difference is operational policy (never move immutable tags, move
  channel tags deliberately).
- Stable must not be moved during build. Stable movement is a separate,
  explicit post-test step.

## Operator/CI handoff

The canonical handoff path is:

1. `decomk checkpoint build` (produce candidate + build artifact),
2. `decomk checkpoint push` (publish immutable + testing/unstable tags + push artifact),
3. external/manual test gate (outside decomk),
4. `decomk checkpoint tag -m` (promote tested source to channel tags such as `stable`).

The durable contract (command templates, artifact fields, retry rules) is
maintained in `TODO/011-single-path-checkpoints.md` under
`011.7 Operator/CI handoff contract`.

## Relationship to planning artifacts

- `TODO/011-single-path-checkpoints.md`: single-path checkpoint
  implementation plan.
- `TODO/010-codespaces-block-prebuild-profiles.md`: block profile/path
  selection.
- `TODO/007-devpod-gcp-selfhost-migration.md`: broader self-hosting
  migration context.
- `TODO/009-phase-eval-lifecycle-spike.md`: lifecycle evidence baseline.
