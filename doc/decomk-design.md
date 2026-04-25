# decomk Design

This document captures the current decomk execution model after the
tuple-only config correction, lifecycle phase selector work, and
phase-mapped MOTD summary support.

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

`DECOMK_MOTD_PHASES` participates in this same tuple contract. It is parsed as a
strict CSV of `NN:phase` entries (for example
`88:version,93:updateContent,94:postCreate`) and drives optional run-summary
MOTD output.

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

`decomk run` uses `DECOMK_STAGE0_PHASE` as the runtime phase key for mapped MOTD
output.

Failure policy is explicit via `DECOMK_FAIL_NOBOOT`:

- `true` (`1|yes|on`) => stage-0 exits non-zero on failure.
- unset/`false` (`0|no|off`) => stage-0 records diagnostics and returns success.

In continue-boot mode, stage-0 writes deterministic artifacts:

- `<DECOMK_HOME>/stage0/failure/latest-<phase>.marker`
- `<DECOMK_HOME>/stage0/failure/latest-<phase>.log`
- MOTD hint at `/etc/motd.d/80-decomk-stage0` when writable, otherwise fallback
  hint at `<DECOMK_HOME>/stage0/failure/motd.txt`.

## 5.1) Run-summary MOTD contract

When resolved tuples include `DECOMK_MOTD_PHASES`, `decomk run` can write
phase-specific MOTD summaries after make completes:

- runtime phase mapping:
  - `/etc/motd.d/<NN>-decomk-<DECOMK_STAGE0_PHASE>`
- optional version mapping:
  - `/etc/motd.d/<NN>-decomk-version`
  - body begins with a blank line and `decomk version: <value>`

If `/etc/motd.d` is unavailable, decomk writes the same filename under:

- `<DECOMK_HOME>/stage0/failure/<filename>`

Mapping parse errors are reported as run warnings.

## 6) Selftest implications

Selftests keep their target logic in fixture `Makefile` targets and scripts.

- phase hooks are tested via explicit stage-0 phase calls,
- tuple-selector expansion and literal-target fallback are tested via `decomk run`,
- stamp idempotency is tested with repeated stamp probe/verify runs.

No selector translation table is required in the harness.

## 7) `decomk init` identity/default contract

`decomk init` treats remote identity as an **image contract**, not a consumer
`devcontainer.json` contract:

- Producer scaffolding (`decomk init -conf`) writes `DECOMK_REMOTE_USER` /
  `DECOMK_REMOTE_UID` in `.devcontainer/Dockerfile` via `ENV`.
- Consumer scaffolding does not emit `DECOMK_REMOTE_*` in generated
  `devcontainer.json`; stage-0 expects those vars from the image at runtime.
- `updateRemoteUserUID` is emitted as `false` to keep UID behavior deterministic.

Consumer init still clones the producer conf repo, but only to import shared
defaults for:

- `DECOMK_TOOL_URI`
- `DECOMK_HOME`
- `DECOMK_LOG_DIR`

Precedence is:

- for tool/home/log: `CLI > producer > local existing > built-in`
- for `DECOMK_FAIL_NOBOOT`: `CLI > local existing > false`

Lifecycle hook commands remain constants in generated output:

- `updateContentCommand`: `bash .devcontainer/decomk-stage0.sh updateContent`
- `postCreateCommand`: `bash .devcontainer/decomk-stage0.sh postCreate`
