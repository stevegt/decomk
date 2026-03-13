# decomk Selftest Design (Git-Served Tool+Config + Auto Context)

This document captures the current selftest design for `examples/decomk-selftest` after aligning with the isconf-style context model.

## 1) Core objectives

1. Exercise real stage-0 clone/pull paths for both tool and config repos (`DECOMK_TOOL_REPO`, `DECOMK_CONF_REPO`).
2. Keep `postCreateCommand.sh` generic and production-identical.
3. Keep test logic in `decomk.conf`, `Makefile`, and scripts in the pulled config repo.
4. Verify automatic context selection from workspace repo identity (no explicit `-context` in harness).
5. Verify tuple precedence: context tuple overrides DEFAULT tuple, while unrelated DEFAULT tuples remain available.

## 2) Harness architecture

`run.sh` responsibilities:

1. Build a temporary fixture config repo from `fixtures/confrepo/`.
2. Build a temporary bare tool repo from the current decomk checkout.
3. Serve both via a temporary `git daemon` (`git://host.docker.internal:<port>/...`).
3. Provision one DevPod workspace.
4. Inject runtime inputs via `devcontainer.json` (`DECOMK_TOOL_REPO`, `DECOMK_CONF_REPO`, `DECOMK_RUN_ARGS`, path envs).
5. Read container `make.log` and enforce PASS/FAIL markers.

`postCreateCommand.sh` responsibilities:

1. Ensure a `decomk` binary is available in `PATH`:
   - default (`DECOMK_TOOL_MODE=install`): `go install ${DECOMK_TOOL_INSTALL_PKG}`.
   - optional (`DECOMK_TOOL_MODE=clone`): clone/pull `DECOMK_TOOL_REPO`, then run `go install ./cmd/decomk` in that repo.
2. Clone/pull `DECOMK_CONF_REPO` into `${DECOMK_HOME}/conf`.
3. Run `decomk run ...` directly (no `go run` required in stage-0).

It does not contain test assertions; test verdict logic stays in config-repo make/scripts and harness log parsing.

## 2.1) Template/generation model

- Canonical scaffold templates live at:
  - `cmd/decomk/templates/devcontainer.json.tmpl`
  - `cmd/decomk/templates/postCreateCommand.sh.tmpl`
- `decomk init` uses embedded copies of those templates.
- Example stage-0 files are generated from the same template contract via `go generate ./...` (`cmd/stage0gen`).
- Drift is blocked by `cmd/decomk/stage0_sync_test.go` and `go run ./cmd/stage0gen -check`.

## 3) Config + context model under test

Fixture `decomk.conf`:

- `DEFAULT` defines:
  - `TUPLE_VERIFY_TOOL`
  - `TUPLE_VERIFY_CONF`
  - `TUPLE_CONTEXT_OVERRIDE`
  - `TUPLE_DEFAULT_SHARED`
- `decomk` context defines:
  - `TUPLE_CONTEXT_OVERRIDE` (override value)

Expected behavior:

- stage-0 tool repo origin matches `DECOMK_TOOL_REPO`,
- stage-0 config repo origin matches `DECOMK_CONF_REPO`,
- `decomk` context is selected automatically from workspace repo name.
- `TUPLE_CONTEXT_OVERRIDE` resolves to the `decomk` context value (override).
- `TUPLE_DEFAULT_SHARED` remains available from `DEFAULT`.

## 4) Test execution and verdicts

Default harness action args:

- `TUPLE_VERIFY_TOOL`
- `TUPLE_VERIFY_CONF`
- `TUPLE_CONTEXT_OVERRIDE`
- `TUPLE_DEFAULT_SHARED`

Fixture make/scripts emit markers to stdout (captured in `make.log`):

- `SELFTEST PASS tool-repo-origin`
- `SELFTEST PASS conf-repo-origin`
- `SELFTEST PASS context-override`
- `SELFTEST PASS default-tuple-available`

Failure marker format:

- `SELFTEST FAIL ...`

`run.sh` exits non-zero if:

- any required PASS marker is missing, or
- any FAIL marker is present, or
- decomk/make fails before marker validation.
