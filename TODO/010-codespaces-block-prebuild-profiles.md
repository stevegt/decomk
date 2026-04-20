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

## Goal

Allow selecting which container block/profile is prebuilt by choosing a devcontainer path in Codespaces settings, instead of mutating one shared Dockerfile reference.

## Proposed structure

```text
.devcontainer/
  Block00/
    devcontainer.json
    Dockerfile
  Block10/
    devcontainer.json
    Dockerfile
  Block12/
    devcontainer.json
    Dockerfile
```

Each `devcontainer.json` points to its sibling Dockerfile, for example:

```json
{
  "name": "decomk Block00",
  "build": {
    "dockerfile": "Dockerfile",
    "context": "../.."
  }
}
```

## Codespaces profile selection model

For each block, create a separate prebuild configuration in:

- `https://github.com/<owner>/<repo>/settings/codespaces/prebuild_configurations`

and set its devcontainer path to one of:

- `.devcontainer/Block00/devcontainer.json`
- `.devcontainer/Block10/devcontainer.json`
- `.devcontainer/Block12/devcontainer.json`

This path becomes the profile selector for prebuild.

## Notes and constraints

- Keep Dockerfiles independent per block; avoid hidden cross-block coupling.
- Keep block naming stable to avoid breaking prebuild settings references.
- If multiple blocks share logic, use shared `ARG`/`FROM` patterns or include scripts, not manual copy/paste drift.
- Ensure any harness/CLI commands that create codespaces pass the matching `--devcontainer-path`.
- Single-path checkpoint workflow for startup-speed improvement is tracked in `TODO/011-single-path-checkpoints.md` and `doc/image-management.md`.

## Subtasks

- [ ] 010.1 Create `.devcontainer/Block00`, `.devcontainer/Block10`, `.devcontainer/Block12` with `devcontainer.json` + `Dockerfile`.
- [ ] 010.2 Decide common build context strategy (`../..` vs narrower contexts) and apply consistently.
- [ ] 010.3 Add/adjust scripts/harness flags to accept block-specific `--devcontainer-path`.
- [ ] 010.4 Configure one Codespaces prebuild profile per block path in repo settings.
- [ ] 010.5 Validate each block by creating a codespace from its path and recording outcomes.
- [ ] 010.6 Document ongoing maintenance rules (how to add new `BlockNN` profile safely).

## Day-in-the-life stories

### DITL-010-DEVOPS-01 - DevOps engineer curating block prebuild profiles

At the start of the week, the DevOps engineer needs to reduce prebuild and first
boot time as shared setup grows. They open the repo and expect one clear profile
path per block (`.devcontainer/BlockNN/devcontainer.json`) with a local sibling
`Dockerfile` so block ownership is obvious. They run automation and harness
commands with an explicit `--devcontainer-path` to target one block at a time,
then update the matching Codespaces prebuild configuration for that same path.
Their definition of success is that block progression is a predictable path switch
operation that feeds into single-path checkpoints, not hand-edited shared
devcontainer rewrites.

### DITL-010-DEVTEAM-01 - Dev team member consuming block profiles

A dev team member joins a repo, follows the documented block path for that team,
and expects workspace startup to just work. They should not need to reverse
engineer which Dockerfile or profile they are on. Over time, they rely on stable
block names and paths in scripts, onboarding notes, and troubleshooting steps.
When a new `BlockNN` is introduced, they expect clear maintenance rules that
explain how to adopt it safely and what behavior changes to expect. Their
definition of success is that block selection feels predictable and does not
interrupt day-to-day coding.

## Story-to-subtask traceability

- `010.1` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.2` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.3` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.4` -> `DITL-010-DEVOPS-01`
- `010.5` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
- `010.6` -> `DITL-010-DEVOPS-01`, `DITL-010-DEVTEAM-01`
