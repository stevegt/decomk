# TODO 006 - decomk: user customization via GITHUB_USER

Goal: allow per-user customization in devcontainers without requiring every user
to add stanzas to the shared decomk config repo.

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

Add a **user-local overlay config** that is loaded automatically and can define
contexts keyed by `GITHUB_USER`.

Layering / precedence ("last wins"):
1. config repo `<DECOMK_HOME>/conf/decomk.conf` (lowest)
2. user overlay `<DECOMK_HOME>/user/decomk.conf` (and sibling `decomk.d/*.conf`)
3. explicit `-config` / `DECOMK_CONFIG` (highest)

Context selection behavior (workspace scan mode):
- Keep current repo-driven key selection.
- Additionally, if `GITHUB_USER` is set and a matching context exists, append a
  `user/<GITHUB_USER>` context key at the highest precedence for the run.

This keeps per-user policy in the container (or bind-mounted volume), not in the
shared config repo.

## Alternative designs to consider

1. Add `GITHUB_USER` as a primary context key in the shared config repo
   (`<GITHUB_USER>:` stanzas).
   - Requires per-user edits to shared repo; conflicts with the goal above.

2. Add a separate "user stage" that runs after the main make run (e.g. a second
   make invocation or a dedicated `USER_INSTALL` variable/target).
   - Pros: explicit separation between shared and personal installs.
   - Cons: more moving parts; harder to reason about precedence; may duplicate
     stamp/log handling.

## Subtasks

- [ ] 006.1 Add a user config search path (`<DECOMK_HOME>/user/decomk.conf`).
- [ ] 006.2 Add `-user-config` flag + `DECOMK_USER_CONFIG` env override.
- [ ] 006.3 Update `loadDefs` to merge config repo + user overlay + explicit config.
- [ ] 006.4 Extend workspace context selection to optionally include `user/<GITHUB_USER>`.
- [ ] 006.5 Document the feature in `README.md` (layout, precedence, examples).
- [ ] 006.6 Add unit tests for layering + key selection (no sudo/network).

