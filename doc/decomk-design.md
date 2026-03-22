# decomk Design (Selftest + isconf-Aligned Selector Semantics)

This document captures the current design direction discussed for decomk selftest behavior after reviewing `doc/isconf-design.md`.

It focuses on:

- how decomk action arguments should be interpreted (isconf-aligned),
- how `examples/decomk-selftest` should use `decomk.conf` and `Makefile`,
- how to avoid selector translation tables while still testing tuple-selector behavior.

---

## 1) Design Goals

1. Keep decomk behavior aligned with isconf’s model:
   - context expansion produces tuples and tokens,
   - action args can map through tuple variables,
   - unknown action args fall back to literal targets.
2. Avoid maintaining selector translation tables in the harness.
3. Make selftest easy to reason about and deterministic.
4. Keep policy in `decomk.conf`, execution graph in `Makefile` + scripts.

---

## 2) Core decomk Action-Arg Model

For `decomk run [ARGS...]`:

1. Resolve context keys (typically `DEFAULT` plus discovered keys, or explicit `-context`).
2. Expand macros to tokens.
3. Partition tokens into:
   - tuples (`NAME=value`),
   - config target tokens.
4. Select final targets:
   - if `ARGS` are present:
     - if arg matches an effective tuple variable name, expand to that tuple’s value (split on whitespace),
     - otherwise treat arg as a literal make target.
   - if no `ARGS`:
     - use config target tokens if present,
     - else fallback to `INSTALL` tuple if present,
     - else make default goal.

This is the same shape documented in `doc/isconf-design.md`, adapted to decomk.

## 2.1) Environment contract (`env.sh` == make env)

- `decomk run` writes `${DECOMK_HOME}/env.sh` and uses the same canonical tuple
  list to invoke make.
- Canonical order is:
  1. incoming `DECOMK_*` passthrough vars,
  2. resolved config tuples,
  3. decomk-computed vars (last-wins).
- Tuple value sentinel `NAME=$` resolves from incoming env; if missing, decomk
  falls back to an earlier tuple assignment for `NAME`; if no fallback exists,
  run/plan fails fast.
- This keeps env exports and runtime make/recipe env behavior consistent even
  when make runs via sudo.

---

## 3) Selftest Design Decisions

## 3.1 Primary selector mode: literal targets

Primary selftest execution should use literal Makefile target names as decomk args:

- `all`
- `root_hook_owner_inferred`
- `non_root_default_make_as_root`
- `no_sudo_expect_fail`
- `no_sudo_make_as_user`

That means no primary selector mapping table in `decomk.conf`.

## 3.2 Tuple-selector coverage still required

Even with literal-primary mode, selftest must explicitly exercise tuple selector expansion:

- tuple-only runs (arg maps to tuple value),
- mixed runs (tuple selector + literal target in same invocation),
- non-`DEFAULT` context expansion path.

## 3.3 No translation table policy

The harness should not keep a selector→scenario lookup for expected outcomes.
Instead, scenario scripts should record what actually executed, and result verification should use that execution manifest.

---

## 4) `decomk.conf` and `Makefile` Responsibilities

## 4.1 `decomk.conf` (policy/config layer)

`DEFAULT` should contain:

- runtime fixture tuples (paths, roots, repo locations),
- optional tuple selectors used only for tuple-path tests,
- no required mapping for literal-primary selectors.

Example pattern:

```conf
DEFAULT: SELFTEST_RESULTS_DIR='...'
  SELFTEST_STATE_ROOT='...'
  ...
  TUPLE_ALL='all'
  TUPLE_ROOT='root_hook_owner_inferred'
  TUPLE_PAIR='root_hook_owner_inferred no_sudo_expect_fail'

SELFTEST_CONTEXT_EXTRA: CTX_TUPLE='non_root_default_make_as_root'
```

## 4.2 `Makefile` (execution graph layer)

`Makefile` defines the executable target graph:

- `all` depends on scenario targets,
- scenario targets call scenario scripts,
- scripts write per-scenario machine-readable results.

No selector mapping logic is required in `Makefile`; target names are the selectors for literal-primary runs.

---

## 5) Harness Contract

## 5.1 `run.sh`

- Accepts raw decomk args as positional passthrough.
- Writes those args into `.devcontainer/SELFTEST_DECOMK_ARGS` for the hook.
- Brings up one DevPod workspace and verifies overall + per-scenario results.
- Uses `all` by default when no args are provided.

Compatibility note:

- `--subtests` / `--scenario` may be retained as comma-separated aliases during migration, but they should map directly to action args (no lookup table).

## 5.2 `postCreateCommand.sh`

- Seeds temporary fixture config repo.
- Replaces runtime placeholders in `decomk.conf`.
- Runs `go run ./cmd/decomk run <passed-args...>`.
- Validates execution using:
  - `overall.env`,
  - per-scenario `result.env`,
  - an execution manifest (e.g., `executed.list`) emitted by scenario scripts.

This avoids hardcoded selector-expansion tables in the hook.

---

## 6) Test Matrix Requirements

Selftest should cover all three selector forms:

1. **Literal-only**
   - `decomk run all`
2. **Tuple-only**
   - `decomk run TUPLE_ALL`
3. **Mixed tuple + literal**
   - `decomk run TUPLE_ROOT no_sudo_make_as_user`
4. **Non-`DEFAULT` context tuple**
   - `decomk run -context SELFTEST_CONTEXT_EXTRA CTX_TUPLE`

Success criteria:

- decomk exits 0,
- every executed scenario has `OUTCOME=PASS`,
- overall result is `PASS`.

---

## 7) Why this design

This keeps decomk selftest consistent with isconf’s architecture:

- tuples remain first-class for action-variable semantics,
- literal targets remain first-class for direct invocation,
- config stays declarative,
- execution stays in make targets/prereqs,
- no fragile translation table is needed.

---

## 8) Non-goals

- This design does not change decomk core parser grammar.
- This design does not require making action args expand arbitrary top-level macro keys by default.
- This design does not require separate containers per scenario; one container per harness invocation remains acceptable.

AI:

This is a very thorough design document for the decomk selftest behavior. It looks like the goal is to closely align decomk’s behavior with that of isconf by:

• Treating tuples as first‐class citizens so that action arguments can be expanded to their tuple values when the arg matches a tuple variable name, while still allowing literal make targets.

• Making the selftest harness easy to reason about by:
 – Using literal target names as the primary selectors (e.g., “all”, “root_hook_owner_inferred”, etc.),
 – Avoiding a mapping/translation table for selectors, and
 – Ensuring that scenario scripts record their execution so that results can be validated against an execution manifest.

• Separating policy (in decomk.conf) from execution (in the Makefile and scripts). For example:
 – decomk.conf defines tuples like TUPLE_ALL, TUPLE_ROOT, etc., alongside runtime values for fixtures,
 – The Makefile contains the target graph that invokes the actual scenario scripts.

• Supporting three key test invocation modes:
 1. Literal-only (direct Make targets),
 2. Tuple-only (where an action arg is a tuple that expands to specific value(s)), and
 3. Mixed (combining tuple selectors with literal make targets).
 An additional path supports non-DEFAULT contexts via the -context flag.

Additionally, the harness (as realized in run.sh and postCreateCommand.sh) captures raw decomk args, seeds the config repository with runtime values, runs the decomk command, and validates results based on both overall and per-scenario outputs.

Overall, the design strives to:
 – Maintain consistency with isconf’s tuple expansion model,
 – Avoid brittle translation tables,
 – Keep the configuration declarative (in decomk.conf), and
 – Delegate the actual execution flow to the Makefile and harness scripts.

Does this summary capture your intent correctly? Do you have any specific points you’d like to discuss further, or are you looking for feedback, validation of certain design choices, or assistance with implementation?
