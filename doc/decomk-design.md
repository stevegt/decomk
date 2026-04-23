# decomk Design

This document captures the current decomk execution model after the
tuple-only config correction and lifecycle phase selector work.

## 1) Core model

`decomk` keeps three concerns separate:

1. **Identity selection**: pick config keys (`DEFAULT`, repo keys, or explicit `-context`).
2. **Tuple resolution**: expand selected keys into `NAME=value` tuples.
3. **Action selection**: use positional args to decide which make targets to run.

`decomk.conf` is the policy layer; `Makefile` is the execution graph.

## 2) `decomk.conf` contract (strict)

RHS tokens in `decomk.conf` must be one of:

- `NAME=value` tuples, or
- macro references to other keys defined in `decomk.conf`.

Bare RHS tokens that are neither tuples nor defined keys are rejected as
configuration errors with key+token context.

This removes accidental "implicit target tokens" from config and aligns with the
isconf tuple/macro pattern.

## 3) `plan` / `run` action args

`decomk plan` and `decomk run` require at least one positional action arg.

For each action arg:

- if it matches a resolved tuple variable name, decomk splits that tuple value on
  whitespace and appends those words as make targets;
- otherwise decomk treats the arg as a literal make target.

There is no no-arg fallback behavior.

## 4) Canonical environment contract

`decomk` computes one canonical tuple stream and uses it for both:

- env export file (`<DECOMK_HOME>/env.sh`), and
- make invocation argv/env.

Ordering is:

1. incoming `DECOMK_*` passthrough vars,
2. resolved config tuples,
3. decomk-computed tuples (last-wins).

Tuple value sentinel `NAME=$` means: take incoming env `NAME`; if unset, reuse an
earlier tuple assignment for `NAME`; otherwise fail.

## 5) Stage-0 lifecycle contract

Generated devcontainer hooks call:

- `bash .devcontainer/decomk-stage0.sh updateContent`
- `bash .devcontainer/decomk-stage0.sh postCreate`

`decomk-stage0.sh`:

1. ensures a `decomk` binary is available (`DECOMK_TOOL_URI`),
2. syncs config repo (`DECOMK_CONF_URI`),
3. exports `DECOMK_STAGE0_PHASE`,
4. runs `decomk run <action-args>`.

Action args default to the lifecycle phase selector (`updateContent` or
`postCreate`). Optional extra args passed to `decomk-stage0.sh` override that
default selector list.

## 6) Selftest implications

Selftests keep their target logic in fixture `Makefile` targets and scripts.

- phase hooks are tested via explicit stage-0 phase calls,
- tuple-selector expansion and literal-target fallback are tested via `decomk run`,
- stamp idempotency is tested with repeated stamp probe/verify runs.

No selector translation table is required in the harness.
