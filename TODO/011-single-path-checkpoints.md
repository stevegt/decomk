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
- [ ] 011.3 Implement `decomk checkpoint` (v1) to run prebuild/common lifecycle with `devcontainer up --prebuild`, then commit a checkpoint image.
- [ ] 011.3.1 Add `checkpoint` subcommand wiring in `cmd/decomk/main.go` and usage text updates.
- [ ] 011.3.2 Add `cmdCheckpoint(args, stdout, stderr)` in a new `cmd/decomk/checkpoint.go` using `flag.FlagSet`.
- [ ] 011.3.3 Require `--tag`; support `--workspace` (default `.`), optional devcontainer config override, and `--keep-container`.
- [ ] 011.3.4 Execute `devcontainer up --prebuild` and parse result metadata needed to identify the created container.
- [ ] 011.3.5 Commit container to caller tag via Docker and inspect the resulting image ID.
- [ ] 011.3.6 Emit a machine-readable JSON result on stdout (success/error, tag, container/image identifiers, cleanup status).
- [ ] 011.3.7 Default to container cleanup after checkpoint, with explicit retain behavior when `--keep-container` is set.
- [ ] 011.3.8 Add focused unit tests with command stubs for success, required-flag errors, prebuild failures, commit failures, and cleanup failures.
- [ ] 011.4 Enforce phase separation so checkpoint images exclude runtime/user-phase (`postCreate`) side effects.
- [ ] 011.5 Add deterministic pinning checks (base digest, package/tool versions, git refs, and other mutable inputs) so checkpoint behavior remains stable.
- [ ] 011.6 Add same-path verification evidence capture (lifecycle markers/command traces) to prove checkpoints were created through the shared `updateContent` flow.
- [ ] 011.7 Add repeatable operator/CI entrypoints and documentation for checkpoint creation and block handoff.
- [ ] 011.8 Execute acceptance runs and record evidence that checkpointed block progression removes repeated setup work from subsequent prebuild/first-boot paths.

## 011.3 v1 command contract

- Command: `decomk checkpoint --tag <image:tag> [flags]`
- Runner model: prebuild/common lifecycle only (`devcontainer up --prebuild`) for this phase.
- Output: JSON on stdout (not files by default).
- Cleanup: remove checkpoint container by default; keep only with `--keep-container`.
- Deferred by design: numeric speed SLOs and comparator-heavy parity tooling.

## Acceptance criteria

- Checkpoint runs and prebuild runs both execute the same
  `updateContent -> decomk run` path for shared setup.
- Checkpoint images exclude runtime/user-phase (`postCreate`) effects.
- Block progression workflow (checkpoint -> next block base) is
  documented and runnable without hidden/manual steps.
- Evidence artifacts show that repeated shared setup work is shifted into
  checkpoint images for later prebuild/first-boot flows.

## Day-in-the-life stories

### DITL-011-DEVOPS-01 - DevOps engineer maintaining fast block startup

The DevOps engineer sees that prebuild and first boot are getting
slower as shared setup grows. They run `decomk checkpoint --tag
<target>` to bake the current shared setup into a block image using
the same `updateContent -> decomk` path used during prebuild. They
then point the next block's base image at that checkpoint so later
runs skip repeated setup work. By default, checkpoint should
automatically remove the temporary checkpoint container after image
commit. On failure they can run with `--keep-container` to retain the
container for diagnosis. They need JSON output to explicitly report
whether cleanup happened or a container was retained (including the
relevant container ID). Their definition of success is simple: later
prebuild and first-boot flows do less repeated work because checkpoint
layers already contain it.

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
