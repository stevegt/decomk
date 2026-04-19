# TODO 006 - decomk: user customization via GITHUB_USER

## Decision Intent Log

ID: DI-006-20260416-222900
Date: 2026-04-16 22:29:00
Status: active
Decision: Treat `GITHUB_USER` as runtime-only identity data and tie user customization to the runtime `postCreate` phase, while reserving `updateContent` for prebuild/common work where `GITHUB_USER` can be empty.
Intent: Prevent user customization logic from depending on prebuild-time identity that is not consistently available, and keep the lifecycle model explicit for both hosted and self-hosted paths.
Constraints: Maintain repo/context resolution behavior for shared policy, avoid password prompts, and keep per-user customization optional.
Affects: `TODO/006-decomk-user-customization-via-github-user.md`, stage-0 lifecycle templates, selftest lifecycle assertions.

ID: DI-006-20260419-114524
Date: 2026-04-19 11:45:24
Status: active
Decision: Use platform-native dotfiles support as the default path for personal user setup; keep decomk focused on deterministic repo/org policy and optional user-phase make targets.
Intent: Avoid rebuilding per-platform dotfiles behavior in decomk, reduce user handholding, and preserve a single decomk execution model with stamps/logs for policy actions.
Constraints: Keep `updateContent` prebuild-safe and shared-only, keep runtime user actions in `postCreate`, avoid interactive sudo/password prompts, and defer `GITHUB_USER -> USER` fallback for now.
Affects: `TODO/006-decomk-user-customization-via-github-user.md`, stage-0 templates, selftests, README lifecycle guidance.
Supersedes: DI-006-20260416-222900

ID: DI-006-20260419-115209
Date: 2026-04-19 11:52:09
Status: active
Decision: Make the per-user bootstrap helper non-optional for TODO 006 and explicitly choose its placement as a required design decision.
Intent: Ensure there is one clear, testable mechanism for repo/user policy handoff instead of leaving helper behavior as a future optional extension.
Constraints: Keep platform dotfiles as the default for personal dotfiles, keep decomk core free of a generic dotfiles engine, run helper logic through make/stamps/logs, and lock helper placement (decomk subcommand vs separate decomk-repo binary vs conf-repo artifact vs other) before implementation.
Affects: `TODO/006-decomk-user-customization-via-github-user.md`, TODO 006 subtasks, user customization design docs.
Supersedes: DI-006-20260419-114524

Goal: allow per-user customization in devcontainers without requiring every user
to add stanzas to the shared decomk config repo, while minimizing setup friction.

Problem statement:
- In many devcontainer images (including Codespaces), the in-container Unix user
  is often a generic account (e.g. `codespace`, `vscode`). Using `$USER` as a
  context key does not provide per-human customization.
- Devcontainers typically expose `GITHUB_USER` (GitHub login) which is a better
  stable identity for "who is the developer".
- We want to avoid designs that:
  - require a password prompt (no interactive sudo)
  - require each developer to have write access to the shared config repo just
    to add their own overrides

## Proposed design (recommended)

Use a **platform-first** model:
- Let the platform handle personal dotfiles bootstrap (Codespaces/DevPod/devcontainer dotfiles support).
- Keep decomk responsible for deterministic shared policy and repeatable make execution.
- Keep user-specific decomk actions as normal make targets in runtime phase (`postCreate`) rather than adding a decomk-internal dotfiles subsystem.
- Require a helper path for per-user bootstrap orchestration, with placement explicitly decided and documented.

Lifecycle contract:
1. `updateContent`: shared and prebuild-safe actions only; no user-personal logic.
2. `postCreate`: runtime user-phase actions; may depend on user identity/runtime state.
3. All decomk-managed actions continue to run through make targets so stamps/logging remain consistent.

### Required helper placement decision

TODO 006 requires a helper mechanism for per-user bootstrap behavior, executed
via the main Makefile so it stays inside decomk stamps/logging semantics.
Before implementation, choose and lock one placement:
- decomk subcommand
- separate binary built from the decomk repo
- helper artifact/script sourced from the conf repo
- another location with explicit rationale

The chosen placement must be documented with invocation contract (inputs,
outputs, error handling, and stamp interactions).

## Alternative designs to consider

1. Decomk-managed dotfiles clone/script execution in core stage-0.
   - Rejected as default due to platform drift and extra support burden.

2. User-local overlay config under `<DECOMK_HOME>/user/decomk.conf`.
   - Deferred as optional future path; no longer the primary recommendation.

## Deferred / out of scope

- `GITHUB_USER -> USER` fallback is deferred and is not part of TODO 006 implementation.
- A decomk-owned dotfiles engine (clone + script dispatch) is out of scope for this TODO.

## Superseded subtasks (overlay track)

- [x] 006.1 Add a user config search path (`<DECOMK_HOME>/user/decomk.conf`). (Superseded by DI-006-20260419-114524; not implemented)
- [x] 006.2 Add `-user-config` flag + `DECOMK_USER_CONFIG` env override. (Superseded by DI-006-20260419-114524; not implemented)
- [x] 006.3 Update `loadDefs` to merge config repo + user overlay + explicit config. (Superseded by DI-006-20260419-114524; not implemented)
- [x] 006.4 Extend workspace context selection to optionally include `user/<GITHUB_USER>`. (Superseded by DI-006-20260419-114524; not implemented)
- [x] 006.5 Document the feature in `README.md` (layout, precedence, examples). (Superseded by DI-006-20260419-114524; not implemented)
- [x] 006.6 Add unit tests for layering + key selection (no sudo/network). (Superseded by DI-006-20260419-114524; not implemented)

## Active subtasks

- [ ] 006.7 Document the platform-first boundary in `README.md` and `doc/decomk-design.md` (platform dotfiles for personal setup; decomk for policy/make orchestration).
- [ ] 006.8 Document phase responsibilities for user customization (`updateContent` shared/prebuild-safe; `postCreate` runtime/user-phase).
- [ ] 006.9 Add/adjust selftests to assert phase separation and that user-phase targets run only in `postCreate`.
- [ ] 006.10 Decide helper placement (decomk subcommand vs separate decomk-repo binary vs conf-repo artifact vs other) and lock the decision in a DI entry with rationale.
- [ ] 006.11 Define the non-optional make-invoked helper contract for per-user bootstrap behavior (inputs, outputs, stamps, logs, failure handling).
- [ ] 006.12 Cross-link this TODO from `TODO/001-decomk-devcontainer-tool-bootstrap.md` and related lifecycle TODOs as the user-customization contract.
