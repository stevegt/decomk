# TODO 010 - codespaces: block-specific devcontainer prebuild profiles

## Decision Intent Log

ID: DI-010-20260417-104109
Date: 2026-04-17 10:41:09
Status: active
Decision: Model build profiles as separate devcontainer trees (`.devcontainer/BlockXX/`) where each tree contains its own `devcontainer.json` and `Dockerfile`, then select the desired profile by choosing that `devcontainer.json` in Codespaces prebuild configuration.
Intent: Make profile selection explicit and operationally simple (path selection) without editing a shared `devcontainer.json` on each switch.
Constraints: Keep each profile runnable directly, keep profile naming stable (`BlockNN`), and document exact prebuild path mapping so CI/Codespaces behavior is deterministic.
Affects: `.devcontainer/Block*/devcontainer.json`, `.devcontainer/Block*/Dockerfile`, Codespaces prebuild configuration settings, selftest/docs that reference devcontainer paths.

ID: DI-010-20260417-135400
Date: 2026-04-17 13:54:00
Status: active
Decision: Track block profile/path selection in TODO 010, while moving freeze/parity implementation and acceptance-gate details to TODO 011 and `doc/image-management.md`.
Intent: Keep TODO 010 focused on profile structure and Codespaces path selection, and avoid mixing it with parity pipeline implementation details.
Constraints: Preserve existing BlockNN path model; keep prebuild profile decisions compatible with TODO 011 parity requirements.
Affects: `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/011-single-path-checkpoints.md`, `doc/image-management.md`, `TODO/TODO.md`.

ID: DI-010-20260419-124830
Date: 2026-04-19 12:48:30
Status: active
Decision: Drive TODO 010 execution with plain-English user stories for both DevOps engineer and Dev team member personas; defer Gherkin conversion.
Intent: Make block profile outcomes explicit for both operators and consumers before implementation details change.
Constraints: Keep the BlockNN path-selection model unchanged, preserve compatibility with TODO 011 freeze/parity work, and avoid introducing behavior changes in this documentation update.
Affects: `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/011-single-path-checkpoints.md`.

ID: DI-010-20260419-130112
Date: 2026-04-19 13:01:12
Status: active
Decision: Replace short user-story bullets with day-in-the-life narratives for both DevOps engineer and dev team member personas in TODO 010.
Intent: Capture operational flow and pain points with realistic narrative context so implementation priorities are easier to validate.
Constraints: Keep block-path selection model unchanged, keep traceability to 010 subtasks explicit, and keep the update documentation-only.
Affects: `TODO/010-codespaces-block-prebuild-profiles.md`.
Supersedes: DI-010-20260419-124830

ID: DI-010-20260419-141822
Date: 2026-04-19 14:18:22
Status: active
Decision: Align TODO 010 with speed-first single-path checkpoint design by referencing TODO 011 as `single-path checkpoints` and focusing profile selection on reducing prebuild/first-boot time through block progression.
Intent: Keep profile selection work tied to the real operational objective (startup speed) without adding separate parity-comparator responsibilities to TODO 010.
Constraints: Preserve existing BlockNN path model, keep story/subtask traceability intact, and keep this update documentation-only.
Affects: `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/011-single-path-checkpoints.md`, `doc/image-management.md`.
Supersedes: DI-010-20260419-130112

ID: DI-010-20260420-160634
Date: 2026-04-20 16:06:34
Status: active
Decision: Treat `.devcontainer/BlockXX/` subdirectories as a considered alternative that is rejected for now; use one canonical `.devcontainer/devcontainer.json` per repo and commit explicit edits when selector policy changes.
Intent: Avoid profile-tree duplication and drift while keeping repo configuration simple and explicit for current checkpoint-channel rollout work.
Constraints: Keep TODO 010 focused on profile selection behavior (not checkpoint internals), preserve compatibility with TODO 011 `build/push/tag` flow, and keep this change documentation-only.
Affects: `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/011-single-path-checkpoints.md`, `doc/image-management.md`.
Supersedes: DI-010-20260419-141822

## Goal

Define how repos select checkpoint channels/pins in devcontainer config
without requiring per-user config edits whenever a new checkpoint image
is published.

## Canonical repo configuration model

- Each repo keeps one canonical devcontainer config:
  - `.devcontainer/devcontainer.json`
- Repo maintainers edit that file explicitly as needed and commit changes.
- Consumer repos should use `image:` references (channel or pinned tag)
  so routine checkpoint rollout does not require changing repo config on
  each checkpoint publish.

## Considered alternatives (rejected for now)

### `.devcontainer/BlockXX/` profile trees

This TODO considered one profile directory per block, for example:

```text
.devcontainer/Block00/devcontainer.json
.devcontainer/Block10/devcontainer.json
```

Rejected for now because it increases duplication/drift risk and adds
maintenance overhead across many profile trees. Current direction is to
keep one canonical `devcontainer.json` per repo and rely on checkpoint
channel/pin selection rather than path switching.

## Notes and constraints

- Keep selector behavior explicit and git-tracked in repo config.
- Avoid requiring users to edit local Codespaces settings to adopt each
  new checkpoint publish.
- Keep checkpoint internals (`build/push/tag`, external test gate, tag
  movement semantics) in TODO 011.
- Single-path checkpoint workflow for startup-speed improvement is
  tracked in `TODO/011-single-path-checkpoints.md` and
  `doc/image-management.md`.

## Subtasks

- [ ] 010.1 Document producer vs consumer repo config expectations (producer may build checkpoints; consumer should use `image:` selector refs).
- [ ] 010.2 Document selector modes for consumers (channel-following vs pinned checkpoint tag) and when each is appropriate.
- [ ] 010.3 Update harness/docs assumptions to canonical `.devcontainer/devcontainer.json` usage (no BlockXX path requirement).
- [ ] 010.4 Document repo-maintainer workflow for explicit `devcontainer.json` edits + commit when selector policy changes are needed.
- [ ] 010.5 Validate that channel-following consumer repos pick up new tested checkpoint tags without per-repo config edits.
- [ ] 010.6 Document maintenance rules for keeping selector policy clear and minimizing config churn.

## Day-in-the-life stories

### DITL-010-DEVOPS-01 - DevOps engineer curating block prebuild profiles

At the start of the week, the DevOps engineer needs to reduce prebuild and first
boot time as shared setup grows. They open the repo and expect one
canonical `.devcontainer/devcontainer.json` that clearly states whether
the repo follows a channel tag or a pinned checkpoint tag. They publish
new checkpoints through TODO 011 flow (`build -> push -> external test ->
tag`) and expect channel-following repos to benefit without per-repo
config edits for each release. Their definition of success is that only
intentional policy changes require editing `devcontainer.json`.

### DITL-010-DEVTEAM-01 - Dev team member consuming block profiles

A dev team member joins a repo, uses the documented channel or pin
selector for that team, and expects workspace startup to just work.
They should not need to
reverse engineer profile trees or switch devcontainer paths manually.
If the repo follows a channel, they should receive tested checkpoint
updates automatically; if pinned, they should stay pinned until
maintainers intentionally update config. Their definition of success is
predictable startup behavior with minimal config churn.

## Story-to-subtask traceability

- `010.1` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.2` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.3` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.4` -> `DITL-010-DEVOPS-01`
- `010.5` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.6` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
