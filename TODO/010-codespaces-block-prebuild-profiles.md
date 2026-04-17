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
Affects: `TODO/010-codespaces-block-prebuild-profiles.md`, `TODO/011-local-freeze-prebuild-parity.md`, `doc/image-management.md`, `TODO/TODO.md`.

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
- Freeze/parity workflow and acceptance gates are tracked in `TODO/011-local-freeze-prebuild-parity.md` and `doc/image-management.md`.

## Subtasks

- [ ] 010.1 Create `.devcontainer/Block00`, `.devcontainer/Block10`, `.devcontainer/Block12` with `devcontainer.json` + `Dockerfile`.
- [ ] 010.2 Decide common build context strategy (`../..` vs narrower contexts) and apply consistently.
- [ ] 010.3 Add/adjust scripts/harness flags to accept block-specific `--devcontainer-path`.
- [ ] 010.4 Configure one Codespaces prebuild profile per block path in repo settings.
- [ ] 010.5 Validate each block by creating a codespace from its path and recording outcomes.
- [ ] 010.6 Document ongoing maintenance rules (how to add new `BlockNN` profile safely).
