# TODO 015 - mob-sandbox prebuild `/var/*` permissions via checkpoint image

## Decision Intent Log

ID: DI-015-20260424-093505
Date: 2026-04-24 09:35:05
Status: active
Decision: Keep decomk canonical state/log roots under `/var/decomk` and `/var/log/decomk`, do not apply interim mob-sandbox-only workarounds, and instead unblock prebuild by validating `decomk checkpoint build` on the producer path, publishing checkpoint tags to `ghcr.io/ciwg/decomk-conf-cswg`, then updating mob-sandbox to consume that image.
Intent: Fix current prebuild failures without weakening the `/var/*` contract or introducing repo-specific bootstrap exceptions that would need cleanup later.
Constraints: No fallback to `$HOME` for this path, no temporary root-user hook mode, no disabled lifecycle hooks; rollout uses immutable + `testing` + `stable` tag progression.
Affects: `decomk-conf-cswg` image build/publish flow, `mob-sandbox/.devcontainer/devcontainer.json`, evidence docs/TODO references in `decomk`.

## Goal

Resolve prebuild failures where stage-0 cannot write `/var/decomk` and
`/var/log/decomk` by moving mob-sandbox onto a producer image that already
creates/chowns those paths for the dev user.

## Scope

In scope:
- Validate producer checkpoint build flow end-to-end.
- Publish checkpoint image tags under `ghcr.io/ciwg/decomk-conf-cswg`.
- Update mob-sandbox to consume the published image tag and re-run prebuild.
- Capture evidence that permission failures are gone.

Out of scope:
- Changing decomk default path contract (`/var/*` remains canonical).
- Adding stage-0 fallback-to-home logic for this issue.
- Temporary mob-sandbox-only root mode or hook disabling.

## Execution repos

- `decomk-conf-cswg`: producer image source (`.devcontainer/Dockerfile`) and checkpoint source workspace.
- `decomk`: checkpoint command implementation/docs, TODO tracking, and evidence links.
- `mob-sandbox`: consumer devcontainer selector update + prebuild verification.

## Subtasks

- [ ] 015.1 [repo: decomk-conf-cswg] Verify producer Dockerfile ownership contract exists and is committed.
  - Confirm `.devcontainer/Dockerfile` includes:
    - `mkdir -p /var/decomk /var/log/decomk`
    - `chown -R vscode:vscode /var/decomk /var/log/decomk`

- [ ] 015.2 [repo: decomk-conf-cswg] Build a local checkpoint candidate image from producer workspace.
  - Run:
    - `decomk checkpoint build -workspace-folder . -config .devcontainer/devcontainer.json -tag ghcr.io/ciwg/decomk-conf-cswg:block00-candidate`
  - Record JSON output artifact and `sourceResolved` digest/image ID.

- [ ] 015.3 [repo: decomk-conf-cswg] Publish candidate to immutable + testing tags.
  - Run:
    - `decomk checkpoint push ghcr.io/ciwg/decomk-conf-cswg:block00-candidate ghcr.io/ciwg/decomk-conf-cswg:block00-<YYYYMMDD> ghcr.io/ciwg/decomk-conf-cswg:testing`
  - Capture push output including destination digests.

- [ ] 015.4 [repo: mob-sandbox] Update consumer devcontainer image selector to checkpoint testing tag.
  - Set `"image": "ghcr.io/ciwg/decomk-conf-cswg:testing"` in `.devcontainer/devcontainer.json`.
  - Keep:
    - `"DECOMK_HOME": "/var/decomk"`
    - `"DECOMK_LOG_DIR": "/var/log/decomk"`
    - `updateContentCommand` and `postCreateCommand` enabled.

- [ ] 015.5 [repo: mob-sandbox] Re-run prebuild and verify permission failures are eliminated.
  - Required negative checks (must not appear):
    - `mkdir: cannot create directory '/var/decomk': Permission denied`
    - `mkdir: cannot create directory '/var/log/decomk': Permission denied`
    - stage-0 synthetic failure-log/marker write failures caused by missing `/var/decomk/stage0/failure`.
  - Required positive checks:
    - stage-0 reaches decomk execution step.
    - no early trap exit due solely to `/var/*` permissions.

- [ ] 015.6 [repo: mob-sandbox] After testing-tag validation, switch to stable-channel policy as needed.
  - Preferred production target: `"image": "ghcr.io/ciwg/decomk-conf-cswg:stable"`.

- [ ] 015.7 [repo: decomk-conf-cswg] Promote tested immutable image to stable.
  - Run:
    - `decomk checkpoint tag -m ghcr.io/ciwg/decomk-conf-cswg:block00-<YYYYMMDD> ghcr.io/ciwg/decomk-conf-cswg:stable`

- [ ] 015.8 [repo: decomk] Record evidence links and cross-reference TODO 011 rollout contract.
  - Add artifact paths/command outputs to this TODO.
  - Link to `TODO/011-single-path-checkpoints.md` operator flow where relevant.

## Evidence requirements

Minimum artifact set:
- Checkpoint build output JSON showing `candidateTag`, `sourceResolved`, and successful build command.
- Checkpoint push output with immutable + testing tag digests.
- Prebuild log from mob-sandbox after image switch showing absence of `/var/*` permission errors.
- Optional post-switch hook run evidence confirming stage-0 reaches `decomk run`.

Suggested local artifact directory:
- `/tmp/mob-sandbox-prebuild-var-fix-<timestamp>/`

## Completion criteria

This TODO is complete when all are true:
- Producer checkpoint candidate is built and published to `ghcr.io/ciwg/decomk-conf-cswg`.
- Mob-sandbox prebuild succeeds without `/var/*` permission failures.
- Stable-tag promotion is complete (or explicitly deferred with rationale and testing evidence).
- Evidence paths are recorded in this file.
