# TODO 017 - channel branch rendering and promotion

## Decision Intent Log

ID: DI-017-20260507-133819
Date: 2026-05-07 13:38:19 -0700
Status: active
Decision: Add a first-class `decomk branch render` command that reads a repo-local `.decomk/channels.json` registry, selects a channel from the current git branch or an explicit flag, renders `.devcontainer/devcontainer.json`, and supports `-check` for stale-render detection.
Intent: Keep `main`, `testing`, and `stable` coherent across devcontainer source selection, conf repo refs, image channel tags, and decomk tool-install refs without relying on branch-local hand edits or `@latest` drift.
Constraints: Preserve existing `decomk checkpoint tag` as the image promotion primitive; keep hooks optional and non-authoritative; require non-`@latest` tool refs for `testing` and `stable`; do not move image-owned runtime path values into conf-repo containerEnv.
Affects: `cmd/decomk/main.go`, `cmd/decomk/branch.go`, `cmd/decomk/branch_test.go`, `docs/thought-experiments/TE-20260507-133819-branch-channel-renderer.md`, `TODO/017-channel-branch-rendering-and-promotion.md`, `TODO/TODO.md`, downstream `.decomk/channels.json` registries.

## Goal

Make branch/channel devcontainer rendering deterministic and decomk-owned so conf repos can validate channel alignment before Codespaces or checkpoint builds read `.devcontainer/devcontainer.json`.

## Subtasks

- [x] 017.1 Run and record the branch-channel renderer thought experiment.
- [x] 017.2 Add the `decomk branch render` command surface.
- [x] 017.3 Add registry parsing, channel auto-detection, rendering, and stale-render checking.
- [x] 017.4 Add tests for channel rendering, `-check`, auto-detection, and `@latest` rejection.
- [ ] 017.5 Apply the registry to initial conf repos and use it in promotion workflows.

## Operator contract

- `decomk branch render [-repo-root .] [-channel auto]` writes the effective `.devcontainer/devcontainer.json`.
- `decomk branch render -check [-repo-root .] [-channel auto]` exits non-zero when the effective devcontainer file is missing or stale.
- `-channel auto` reads the current git branch name. Detached HEAD or non-git directories must use `-channel`.
- Git hooks may call `render -check` locally, but CI and maintainer workflows must treat the decomk command itself as authoritative.
