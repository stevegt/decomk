# TODO 011 - single-path checkpoints

## Decision Intent Log

ID: DI-011-20260417-135200
Date: 2026-04-17 13:52:00
Status: active
Decision: Adopt Option 2 (controlled local/CI freeze pipeline) as the active image-management path for decomk blocks, and explicitly track parity verification against Codespaces prebuild behavior.
Intent: Ensure frozen images include both Dockerfile and decomk `updateContent` effects while using a path we can operate today.
Constraints: Treat direct prebuild export/promotion as unavailable for now; keep lifecycle split (`updateContent` common/prebuild, `postCreate` runtime/user) intact; keep parity-proof model decision deferred to a dedicated bakeoff step.
Affects: `doc/image-management.md`, `TODO/011-single-path-checkpoints.md`, `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/007-devpod-gcp-selfhost-migration.md`, `TODO/TODO.md`.

ID: DI-011-20260418-115406
Date: 2026-04-18 11:54:06
Status: active
Decision: Implement TODO 011.3 as a first-class `decomk checkpoint` subcommand (not an external script), using existing decomk CLI structure (`main.go` switch + `flag.FlagSet`) and running prebuild/common lifecycle via `devcontainer up --prebuild` before checkpointing with Docker.
Intent: Make image checkpointing an operator-facing decomk workflow with one command path that is easy to automate and consistent with the rest of the CLI.
Constraints: Keep TODO 011.1/011.2 deferred; require caller-provided `--tag`; emit JSON to stdout; default to removing the container after checkpoint; no Cobra migration in this phase.
Affects: `cmd/decomk/main.go`, `cmd/decomk/checkpoint.go`, `cmd/decomk/checkpoint_test.go`, `README.md`, `TODO/011-single-path-checkpoints.md`.

ID: DI-011-20260419-124830
Date: 2026-04-19 12:48:30
Status: active
Decision: Drive TODO 011 execution with plain-English user stories for both DevOps engineer and Dev team member personas; defer Gherkin conversion.
Intent: Clarify who uses checkpoint/parity outputs and what outcomes each persona needs before coding the freeze pipeline.
Constraints: Preserve existing `decomk checkpoint` v1 command contract, preserve phase-separation constraints (`updateContent` vs `postCreate`), and avoid behavior changes in this documentation update.
Affects: `TODO/011-single-path-checkpoints.md`, `TODO/010-codespaces-block-prebuild-profiles.md`.

ID: DI-011-20260419-130112
Date: 2026-04-19 13:01:12
Status: active
Decision: Replace short user-story bullets with day-in-the-life narratives for both DevOps engineer and dev team member personas in TODO 011.
Intent: Make the freeze/parity workflow concrete by describing how operators and developers actually experience checkpoint outcomes.
Constraints: Preserve the existing checkpoint v1 contract, keep phase-separation requirements intact, keep subtask traceability explicit, and keep this update documentation-only.
Affects: `TODO/011-single-path-checkpoints.md`.
Supersedes: DI-011-20260419-124830

ID: DI-011-20260419-141822
Date: 2026-04-19 14:18:22
Status: active
Decision: Reframe TODO 011 around a speed-only objective and single-path checkpoints: use the same `updateContent -> decomk run` path for both prebuilds and checkpoint image builds, and remove parity-comparator/proof-model work from active scope.
Intent: Keep freezing focused on reducing prebuild and first-boot time while preserving reasonable equivalence by construction through one shared execution path.
Constraints: Preserve checkpoint v1 command shape, preserve phase split (`updateContent` included, `postCreate` excluded), avoid introducing new speed-metric gates, and keep this TODO path renamed to `TODO/011-single-path-checkpoints.md`.
Affects: `TODO/011-single-path-checkpoints.md`, `TODO/010-codespaces-block-prebuild-profiles.md`, `doc/image-management.md`, `TODO/TODO.md`.
Supersedes: DI-011-20260419-130112

ID: DI-011-20260420-160634
Date: 2026-04-20 16:06:34
Status: active
Decision: Split checkpoint lifecycle into explicit `build`, `push`, and `tag` subcommands with an external/manual test gate between `push` and stable-tag movement; keep source/tag inputs positional and require explicit `-m` to move existing tags.
Intent: Prevent accidental stable rollout during image creation, keep release control explicit, and align checkpoint rollout with channel-first testing (`unstable`/`testing` before `stable`).
Constraints: Keep single-path guarantee (`updateContent -> decomk run`) for checkpoint builds, keep stamp carry behavior, allow source as digest/ref/image ID, and keep test orchestration out of decomk for now.
Affects: `TODO/011-single-path-checkpoints.md`, `TODO/010-codespaces-block-prebuild-profiles.md`, `doc/image-management.md`, `README.md`.
Supersedes: DI-011-20260419-141822

ID: DI-011-20260420-162554
Date: 2026-04-20 16:25:54
Status: active
Decision: Implement `decomk checkpoint build|push|tag` as first-class CLI subcommands with JSON output, default fail-on-existing-tag semantics, explicit `-m` tag-move override, source resolution via inspect-then-pull fallback, and default checkpoint-container cleanup with `--keep-container` diagnostics escape hatch.
Intent: Deliver TODO 011.3 as a usable operator workflow now, with machine-readable artifacts for external release/test gates and explicit safeguards against accidental channel-tag overwrite.
Constraints: Keep using `devcontainer up --prebuild` for build lifecycle execution; keep release test gate external/manual; preserve positional `<source> <tag...>` contract for push/tag.
Affects: `cmd/decomk/main.go`, `cmd/decomk/checkpoint.go`, `cmd/decomk/checkpoint_test.go`, `README.md`, `TODO/011-single-path-checkpoints.md`.

ID: DI-011-20260420-184053
Date: 2026-04-20 18:40:53
Status: active
Decision: Complete TODO 011.7 as documentation-only by defining a canonical operator/CI handoff contract (narrative + command templates + artifact contract) for `build -> push -> external test -> tag`, and explicitly defer 011.4, 011.6, and 011.8.
Intent: Give operators and CI maintainers one repeatable, auditable rollout playbook without introducing new scripts or workflow code in this step.
Constraints: Keep external test gate outside decomk command execution; keep 011.4/011.6/011.8 open and deferred; keep command contract aligned with implemented checkpoint JSON outputs.
Affects: `TODO/011-single-path-checkpoints.md`, `doc/image-management.md`, `README.md`.

## Goal

Implement single-path checkpoints that reduce prebuild and first-boot
time by baking shared setup into block images through the same
`updateContent -> decomk run` path used for prebuild.

## Background

See `doc/image-management.md` for rationale, single-path design
constraints, and relationship to other TODOs.

## Scope

In scope:

- Define and enforce a single-path checkpoint contract.
- Implement a freeze runner that executes Dockerfile + decomk
  `updateContent` phase and commits a checkpoint image.
- Document how each new block checkpoint is used to shorten later
  prebuild and first-boot work.
- Provide operator/CI workflows and evidence capture for checkpoint
  runs.

Out of scope:

- Direct export/promotion of opaque Codespaces prebuild artifacts.
- Runtime/user customization (`postCreate`) in frozen shared block
  images.
- Dedicated parity-comparator/proof-model tooling.
- GCP rollout execution details (tracked in TODO 007).

## Dependencies and links

- Lifecycle evidence baseline: `TODO/009-phase-eval-lifecycle-spike.md`
- Block profile selection model: `TODO/010-codespaces-block-prebuild-profiles.md`
- Self-host migration track: `TODO/007-devpod-gcp-selfhost-migration.md`

## Subtasks

- [ ] 011.1 Define canonical single-path checkpoint contract (prebuild + checkpoint both execute the same `updateContent -> decomk run` flow with identical input surfaces).
- [ ] 011.2 Define block progression workflow (`BlockNN` checkpoint becomes `FROM` base for later blocks) and document operator handoff points.
- [x] 011.3 Implement checkpoint command family (`build`, `push`, `tag`) with explicit release gate between push and stable tagging.
- [x] 011.3.1 Add checkpoint subcommand routing in `cmd/decomk/main.go` and usage text for `checkpoint build`, `checkpoint push`, and `checkpoint tag`.
- [x] 011.3.2 Implement `checkpoint build` handler to run prebuild/common lifecycle (`devcontainer up --prebuild`), commit local candidate image, and emit machine-readable JSON output.
- [x] 011.3.3 Implement `checkpoint push [-m] <source> <tag...>` using positional args (source digest/ref/image ID + one or more destination tags).
- [x] 011.3.4 Implement `checkpoint tag [-m] <source> <tag...>` for retagging tested remote candidates to channel tags (including `stable`) without rebuild.
- [x] 011.3.5 Enforce default fail-on-existing-tag behavior; require explicit `-m` to move existing channel tags.
- [x] 011.3.6 Include source-resolution and tag results in JSON output so external test/release tooling can consume build/push/tag artifacts.
- [x] 011.3.7 Keep temporary checkpoint container cleanup explicit for `build` (`--keep-container` for diagnostics, default cleanup otherwise).
- [x] 011.3.8 Add focused unit tests for build/push/tag success and failure paths (source resolution errors, tag collision without `-m`, registry/tag move errors, cleanup behavior).
- [ ] 011.4 Enforce phase separation so checkpoint images exclude runtime/user-phase (`postCreate`) side effects. (Deferred in this pass; see DI-011-20260420-184053.)
- [ ] 011.5 Add deterministic pinning checks (base digest, package/tool versions, git refs, and other mutable inputs) so checkpoint behavior remains stable.
- [ ] 011.6 Add same-path verification evidence capture (lifecycle markers/command traces) to prove checkpoints were created through the shared `updateContent` flow. (Deferred in this pass; see DI-011-20260420-184053.)
- [x] 011.7 Add repeatable operator/CI entrypoints and documentation for `build -> push -> external test -> tag` block handoff.
- [ ] 011.8 Execute acceptance runs and record evidence that checkpointed block progression removes repeated setup work from subsequent prebuild/first-boot paths. (Deferred in this pass; see DI-011-20260420-184053.)

## 011.3 command contract

- `decomk checkpoint build [flags]`
  - runs prebuild/common lifecycle and creates a local candidate image.
  - outputs JSON with source digest/ref metadata for downstream steps.
- `decomk checkpoint push [-m] <source> <tag...>`
  - publishes one resolved source (digest/ref/image ID) to one-or-more tags.
  - typically used for immutable + `testing`/`unstable` tags.
- `decomk checkpoint tag [-m] <source> <tag...>`
  - retags an already-published, tested source to channel tags (including `stable`) without rebuild.
- move semantics:
  - default fail if destination tag already exists;
  - require explicit `-m` to move existing tags.
- release sequencing:
  - decomk handles `build` and `push`/`tag`;
  - test gate between `push` and stable `tag` is external/manual.
- deferred by design:
  - numeric speed SLOs and full test orchestration remain outside this TODO.

## 011.7 Operator/CI handoff contract

This section defines what the checkpoint handoff is for: to give DevOps
engineers and CI jobs a repeatable, auditable promotion flow where image
creation/publishing is automated through decomk, external validation stays
outside decomk, and stable channel movement is explicit and intentional.

### Canonical flow

1. **Build candidate**: create a local candidate image using prebuild/common phase.
2. **Push candidate tags**: publish immutable + testing/unstable tags.
3. **External/manual test gate**: run validation outside decomk.
4. **Tag promoted channel**: move channel tags (including `stable`) only after tests pass.

### Command templates

```bash
# Step 1: build candidate (writes build JSON artifact)
decomk checkpoint build \
  -workspace-folder "${WORKSPACE_FOLDER:-.}" \
  -config "${DEVCONTAINER_CONFIG:-.devcontainer/devcontainer.json}" \
  -tag "${CANDIDATE_TAG}" \
  > "${ARTIFACT_DIR}/checkpoint-build.json"

# Step 2: publish immutable + testing/unstable tags (writes push JSON artifact)
decomk checkpoint push \
  "${CANDIDATE_TAG}" \
  "${IMMUTABLE_TAG}" \
  "${TESTING_TAG}" \
  > "${ARTIFACT_DIR}/checkpoint-push.json"

# Step 3: external/manual test gate (outside decomk; contract expects a pass/fail decision)
# Example placeholder:
# ./ci/external-checkpoint-tests.sh "${ARTIFACT_DIR}/checkpoint-push.json"

# Step 4: promote stable channel after tests pass (writes tag JSON artifact)
SOURCE_FOR_PROMOTION="$(jq -r '.sourceResolved' "${ARTIFACT_DIR}/checkpoint-push.json")"
decomk checkpoint tag -m \
  "${SOURCE_FOR_PROMOTION}" \
  "${STABLE_TAG}" \
  > "${ARTIFACT_DIR}/checkpoint-tag.json"
```

### Artifact contract

| Step | Required input | Produced artifact | Required fields consumed by next step |
| --- | --- | --- | --- |
| Build | workspace folder, devcontainer config, candidate tag | `checkpoint-build.json` | `sourceResolved` (optional for audit), `candidateTag` |
| Push | source image/tag + destination tags | `checkpoint-push.json` | `sourceResolved` (promotion source), `tags[].destination`, `tags[].digest` |
| External test | `checkpoint-push.json` + org-specific tests | org-specific test evidence | pass/fail gate decision only |
| Tag | source resolved reference + channel tags | `checkpoint-tag.json` | audit fields: `sourceResolved`, `tags[]` |

### Failure and retry rules

- `push` and `tag` fail if a destination tag already exists unless `-m` is provided.
- Retry `build` safely with a new `-tag` when a candidate is discarded.
- Retry `push` with the same immutable source when publish/network fails before gate.
- Run `tag -m` only after external tests pass; never combine promotion with build.
- Preserve JSON artifacts per run for audit/release traceability.

## Acceptance criteria

- Checkpoint runs and prebuild runs both execute the same
  `updateContent -> decomk run` path for shared setup.
- Checkpoint images exclude runtime/user-phase (`postCreate`) effects.
- Stable tags are moved only after explicit `push` + external test + `tag` workflow.
- Block progression workflow (checkpoint -> next block base) is
  documented and runnable without hidden/manual steps.
- Evidence artifacts show that repeated shared setup work is shifted into
  checkpoint images for later prebuild/first-boot flows.

## Day-in-the-life stories

### DITL-011-DEVOPS-01 - DevOps engineer maintaining fast block startup

The DevOps engineer sees that prebuild and first boot are getting
slower as shared setup grows. They run `decomk checkpoint build` to
create a local candidate image via the same `updateContent -> decomk`
path used during prebuild, then `decomk checkpoint push <source>
<immutable> <testing|unstable>` to publish testable tags. After an
external/manual test gate passes, they run `decomk checkpoint tag -m
<source> stable` to move stable explicitly. They need JSON output and
command traces that show which source digest was built/pushed/tagged.
By default, `build` should clean up the temporary checkpoint container;
on failure they can retain it with `--keep-container` for diagnosis.
Their definition of success is simple: later prebuild and first-boot
flows do less repeated work because checkpoint layers already contain it.

### DITL-011-DEVTEAM-01 - Dev team member benefiting from checkpointed blocks

A dev team member starts work expecting the shared block image to behave like the
documented environment. Because shared setup is frozen into checkpointed blocks,
their workspace reaches ready state faster and first boot does less repetitive
work. If setup issues appear, they can use the documented checkpoint and
prebuild entrypoints to understand what shared setup path ran, without needing to
reverse engineer separate tooling paths. Their definition of success is faster
startup and predictable shared setup behavior.

## Story-to-subtask traceability

- `011.1` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.2` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.3` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.3.1` -> `DITL-011-DEVOPS-01`
- `011.3.2` -> `DITL-011-DEVOPS-01`
- `011.3.3` -> `DITL-011-DEVOPS-01`
- `011.3.4` -> `DITL-011-DEVOPS-01`
- `011.3.5` -> `DITL-011-DEVOPS-01`
- `011.3.6` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.3.7` -> `DITL-011-DEVOPS-01`
- `011.3.8` -> `DITL-011-DEVOPS-01`
- `011.4` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.5` -> `DITL-011-DEVOPS-01`
- `011.6` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.7` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
- `011.8` -> `DITL-011-DEVOPS-01`, `DITL-011-DEVTEAM-01`
