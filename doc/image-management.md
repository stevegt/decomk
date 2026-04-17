# Image Management for Decomk Block Freezes

## Purpose

This document records why decomk is choosing a local freeze pipeline
(Option 2) for managed container images, and how that choice connects
to deterministic builds and long-term operations.

This is a background/design document. Implementation work is tracked in
`TODO/011-local-freeze-prebuild-parity.md`.

## Why this matters

For decomk-managed environments, the effective image state is not just
the Dockerfile output. The final state also includes stage-0 lifecycle
execution and Makefile-driven changes during `updateContentCommand`.

That means a Dockerfile-only freeze is incomplete for decomk, and any
image-management strategy must account for:

1. Dockerfile layer changes, and
2. `updateContent` → `decomk-stage0.sh updateContent` → `decomk run` →
   config-repo Makefile effects.

## Why bit-for-bit parity is important

Bit-for-bit identical artifacts are a high-value goal because they give:

- **Deterministic debugging:** a digest mismatch immediately proves the
  runtime is not the same environment.
- **Reliable promotion/rollback:** exact artifact identity supports
  confident rollout gates and rollback targeting.
- **Supply-chain clarity:** attestations and provenance remain tied to a
  stable artifact identity.
- **Cross-project consistency:** shared platform blocks behave
  identically across many repositories and teams.

## Lifecycle facts we are designing around

Empirical results captured in `TODO/009-phase-eval-lifecycle-spike.md`
and its evidence files show:

- `updateContentCommand` is the prebuild/common phase.
- `postCreateCommand` is the runtime/user phase.
- `GITHUB_USER` is runtime/user-phase data.

Therefore, frozen shared block images must be based on prebuild/common
phase behavior, not user-phase behavior.

## Options considered

### Option 1: Promote GitHub prebuild artifact directly to GHCR

**Idea:** run Codespaces prebuild, then export/promote that exact
artifact to GHCR as the frozen block image.

**Result:** rejected for now. Current supported/public interfaces do not
provide a direct export/promotion path for the opaque prebuild snapshot.

### Option 2: Controlled local/CI freeze pipeline (selected)

**Idea:** run a controlled pipeline that executes the same intended
build phases (Dockerfile + `updateContent`) and then snapshots the
resulting image under deterministic constraints.

**Result:** selected as the practical path available now.

### Dockerfile-only freeze

**Idea:** treat Dockerfile output as the block image and skip decomk
phase execution.

**Result:** rejected. This does not represent decomk’s actual managed
state model.

## Decision

Use **Option 2** as the active path: controlled local/CI freeze with
strict determinism controls and parity verification.

Because Option 1 is unavailable today, Option 2 is the only viable path
to produce promotable images that include decomk-managed state.

## Determinism and parity strategy (Option 2)

The implementation target is to maximize parity with Codespaces
prebuild outputs while preserving decomk semantics.

Core controls:

1. Pin base images by digest.
2. Pin tool/package versions and source refs.
3. Run stage-0 prebuild phase explicitly via
   `.devcontainer/decomk-stage0.sh updateContent`.
4. Exclude user-phase (`postCreate`) side effects from frozen blocks.
5. Capture and compare canonical parity artifacts (digests/manifests,
   plus decomk lifecycle markers).
6. Fail fast on drift outside approved categories.

## Known constraint and deferred choice

The exact parity proof model is deferred to the implementation bakeoff
in TODO 011. We are not locking that proof model in this background
document.

## Relationship to other planning artifacts

- `TODO/011-local-freeze-prebuild-parity.md`: implementation plan for
  Option 2.
- `TODO/010-codespaces-block-prebuild-profiles.md`: profile/path
  selection for block-specific prebuild configs.
- `TODO/007-devpod-gcp-selfhost-migration.md`: broader self-hosting
  migration context.
- `TODO/009-phase-eval-lifecycle-spike.md`: lifecycle evidence baseline.
