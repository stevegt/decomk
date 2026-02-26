# TODO 004 - decomk: align flow with isconf (identity → vars → PACKAGES → make)

Goal: align decomk’s user model and execution flow with the proven isconf flow,
while preserving decomk’s devcontainer-specific needs (multi-repo workspaces,
container-scoped stamps, Go implementation).

This TODO is prompted by a comparison with isconf
(`~/lab/isconf2/isconf2i-git`) and the observation that decomk currently
conflates two concepts that isconf keeps separate:
- **identity selection** (which stanza(s) apply), and
- **action selection** (which package groups/targets to run).

## Background: what isconf does (as observed)

Key files in isconf (paths and line references are approximate, per notes):

1) **Identity → choose applicable stanzas**
   - `bin/isconf` uses the current hostname (+ optional `DOMAIN`) to select which
     `conf/hosts.conf` stanzas apply.
   - It always starts with `DEFAULT`, then adds either:
     - `$HOSTNAME.$DOMAIN`, or
     - the short `$HOSTNAME`
     if that key exists. (Example reference: `bin/isconf:56`.)

2) **`conf/hosts.conf` is a recursive macro map**
   - Syntax: `NAME:` expands to macro names plus `VAR=value` tuples.
   - Macros can compose groups (e.g., `HQFT`) that many hosts reuse.
   - When isconf eventually runs `make`, later `VAR=value` tuples on the command
     line override earlier ones. (Example reference: `conf/hosts.conf:35`.)

3) **Macro expansion → a flat list of tuples**
   - `bin/isconf` expands macros using `bin/expandmacro.pl` into a flat list of
     make command-line variable assignments. (Example reference: `bin/isconf:122`.)

4) **Args are *action variables* → PACKAGES**
   - The arguments you pass to isconf (usually from `etc/rc.isconf`) are treated
     as *variable names* like `BOOT`, `CRON`, `INSTALL`.
   - `bin/parseargs.pl` looks up those variable values and turns them into
     `PACKAGES` (a space-separated list of make targets to run).
     (Example references: `etc/rc.isconf:188`, `bin/isconf:124`.)

5) **make runs with vars + targets**
   - make is invoked roughly as:
     - `make -f conf/main.mk <vars...> <targets...>`
   - `conf/main.mk` includes an OS-specific makefile where targets like `Block12`
     and `cron` are defined. (Example references: `conf/main.mk:60`,
     `conf/aix.mk:25`.)

6) **Fallback: if arg isn’t a known variable, treat args as targets**
   - If you pass a literal make target name that isn’t a variable in
     `hosts.conf`, `parseargs.pl` returns nothing and isconf falls back to running
     your args directly as make targets. (Example reference: `bin/isconf:127`.)

## What decomk does today (summary)

Decomk’s current documented flow is (see `README.md`):
- Discover one or more workspace repos (typically under `/workspaces/*`).
- Select a context key per repo.
- Load `decomk.conf` trees with precedence.
- Expand macros recursively into tokens.
- Partition tokens into:
  - tuples (`NAME=value`) and
  - targets (everything else).
- Run `make` with tuples + targets.

Implication: decomk currently expresses **action selection** primarily as *target
tokens in the expansion output*.

## Gaps / mismatches vs isconf’s user model

1) **No explicit “parseargs” stage**
   - isconf: “identity stanza(s) set variables; args select actions; actions map
     to targets via variables”.
   - decomk: “identity stanza(s) expand directly to targets”.

2) **No positional args to select actions**
   - isconf’s interface expects `isconf INSTALL` (or `BOOT CRON …`) and treats
     them as “action variables”.
   - decomk currently has no `decomk run INSTALL` semantics; `run`/`plan` take
     only flags.

3) **Higher risk of silent typos**
   - Because decomk treats unknown non-tuple tokens as targets, a typo in a macro
     token can become a target token and only fail later in make (or worse, run
     the wrong target if a similarly named target exists).
   - isconf’s “vars first, targets derived” model provides a natural place to
     validate action selection (e.g., unknown action vars vs unknown literal
     targets).

4) **Config conventions differ**
   - isconf encourages: contexts expand mostly to macro names + `VAR=value`
     tuples, and actions are variables that contain target lists.
   - decomk currently encourages putting target names directly in `decomk.conf`.

## Recommendations: align decomk with isconf (incrementally)

### Recommendation A: Add an isconf-like “action args → PACKAGES” stage

Define `decomk plan/run [ARGS...]` where each positional arg is processed as:

1) If arg matches the name of an **effective tuple variable** (e.g., `INSTALL`)
   in the resolved env:
   - interpret its *value* as a whitespace-separated target list
   - append those targets to the planned make invocation
   - (optional) expose the final targets list as `PACKAGES=...` (or
     `DECOMK_PACKAGES=...`) for Makefile introspection.

2) Else (arg is not a known variable): treat arg as a **literal make target**
   (isconf fallback).

This mirrors isconf:
- identity selects stanzas → variables,
- args select action variables → packages/targets,
- unknown args behave as literal targets.

### Recommendation B: Shift the *recommended* config style toward action vars

Prefer:
- contexts set action variables like:
  - `INSTALL='install-neovim install-codex'`
  - `CRON='cron'`
  - `BOOT='Block00_base Block10_common'`
- contexts can still include macro tokens for shared tuple bundles, but the
  “what to run” should primarily flow through action variables, not target tokens.

### Recommendation C: Update docs to explain the isconf-aligned mental model

Update `README.md` algorithm section to include:
- identity selection (context key)
- macro expansion
- tuple resolution (last wins)
- **action args → package/target selection**
- make invocation

Provide examples that mirror isconf:

`decomk.conf`:
```conf
DEFAULT: INSTALL='install-neovim install-codex'
repo1: DEFAULT INSTALL='install-mob-consensus'
```

Usage:
```bash
decomk run INSTALL
decomk run install-neovim    # literal target fallback
```

## Subtasks

- [ ] 004.1 Document isconf flow precisely (with examples and invariants).
- [ ] 004.2 Define decomk’s positional-args semantics (`INSTALL` vars vs literal targets).
- [ ] 004.3 Decide default action when no args provided (what does isconf do?)
- [ ] 004.4 Update `README.md` to describe the isconf-aligned flow + examples.
- [ ] 004.5 Implement action-arg parsing (derive targets from effective tuple values).
- [ ] 004.6 Add tests for action-arg parsing and fallback behavior.
