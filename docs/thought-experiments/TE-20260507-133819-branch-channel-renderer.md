# Thought Experiment: Branch Channel Renderer

- **TE ID:** `TE-20260507-133819`
- **Decision under test:** How decomk should keep `main`, `testing`, and `stable` coherent across conf repo refs, checkpoint image tags, and decomk tool-install refs.
- **Related TODO:** `TODO/017-channel-branch-rendering-and-promotion.md`

## Problem statement

The conf repo is both the source for the devcontainer image contract and the configuration repo that stage-0 fetches during boot. A devcontainer decides whether to use `build` or `image` before decomk runs, so runtime logic cannot repair a stale or mismatched `.devcontainer/devcontainer.json` after startup has already begun.

The channel problem is therefore: keep branch, image, conf ref, and tool ref aligned without scattering policy across branch-local hand edits, Dockerfile defaults, and ad hoc environment files.

## Assumptions

1. `main` is the active development branch.
2. `testing` and `stable` are real channels, not just image tags.
3. `testing` and `stable` must not use `DECOMK_TOOL_URI` values ending in `@latest`.
4. Image promotion to `stable` uses `decomk checkpoint tag` on the already-tested `testing` digest.
5. Git hooks may help local developers, but they cannot be trusted as the Codespaces mechanism of record.

## Alternatives

1. **Branch-local effective files**
   - Each branch commits a different effective `.devcontainer/devcontainer.json`.
2. **Runtime-only selection**
   - One committed devcontainer file delegates channel choice to decomk during stage-0.
3. **Shared registry plus selector**
   - One registry describes channels; a decomk renderer materializes the effective devcontainer file before devcontainer startup.

## Scenario analysis

### Scenario A: Main branch Dockerfile development

- **Branch-local effective files:** Simple to understand, but promotion becomes branch-specific file maintenance.
- **Runtime-only selection:** Fails because devcontainer already chose `build` or `image` before decomk starts.
- **Shared registry plus selector:** Keeps `main` renderable as a `build` branch while retaining one source of channel policy.

### Scenario B: Testing validation

- **Branch-local effective files:** Works if humans keep image tag, conf ref, and tool ref aligned.
- **Runtime-only selection:** Cannot validate image selection reliably because the image is already chosen.
- **Shared registry plus selector:** Renders `testing` from explicit registry data and can reject `@latest` before validation.

### Scenario C: Stable promotion after testing passes

- **Branch-local effective files:** Requires careful manual edits or merge exceptions for stable-only channel fields.
- **Runtime-only selection:** Still cannot move the devcontainer source decision early enough.
- **Shared registry plus selector:** Reuses checkpoint retagging for image promotion and renders the stable config from the same registry.

### Scenario D: Local developer checkout

- **Branch-local effective files:** Most obvious when already correct, but stale files are easy to miss after branch switches.
- **Runtime-only selection:** Too late for source selection.
- **Shared registry plus selector:** Git hooks can run `decomk branch render -check` as a guardrail, while the decomk command remains the authoritative check.

## Conclusion

The surviving architecture is **shared registry plus selector**. Add `decomk branch render` to render or check `.devcontainer/devcontainer.json` from `.decomk/channels.json`, with channel auto-detection from the current git branch and explicit `-channel` override for CI or maintainer workflows.

Git hooks are useful local guardrails only. The mechanism of record is the registry plus `decomk branch render -check`, because Codespaces and remote automation cannot be assumed to run local hooks.

## Implications

1. Add `decomk branch render` and `decomk branch render -check`.
2. Keep `decomk checkpoint tag` as the image promotion primitive.
3. Store channel policy in `.decomk/channels.json` in conf repos.
4. Render `.devcontainer/devcontainer.json` before opening the devcontainer or building checkpoint images.
5. Reject `testing` and `stable` configs that use `DECOMK_TOOL_URI` ending in `@latest`.
