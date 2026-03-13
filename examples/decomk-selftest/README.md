# decomk self-test

This directory contains a local DevPod/Docker self-test harness that validates decomk bootstrap behavior end-to-end.

Template note:
- `workspace-template/.devcontainer/devcontainer.json` and `workspace-template/.devcontainer/postCreateCommand.sh` are generated from `cmd/decomk/templates/*`.
- Regenerate from repo root with `go generate ./...` (or `make generate`).

## Current scope

- Local DevPod with Docker provider (automated)
- Codespaces parity checks (planned next stage)
- Remote GCP provider checks (deferred)

## Harness model

1. `run.sh` creates a temporary fixture config repo from `fixtures/confrepo/`.
2. `run.sh` also creates a temporary bare tool repo from the current decomk checkout.
3. `run.sh` serves both repos over `git://` via a temporary local `git daemon`.
4. DevPod starts a workspace from `workspace-template/.devcontainer/`.
5. `postCreateCommand.sh` performs stage-0 bootstrap:
   - ensures a `decomk` binary exists in `PATH` (install-first by default; selftest uses clone mode),
   - clone/pull `DECOMK_CONF_REPO`.
6. `postCreateCommand.sh` runs `decomk run`.
7. `run.sh` reads container make logs and enforces PASS/FAIL markers.
8. `run.sh` then runs two explicit stamp regression invocations:
   - `decomk run TUPLE_STAMP_PROBE`
   - `decomk run TUPLE_STAMP_PROBE TUPLE_STAMP_VERIFY`

## Run

Default run (tuple checks):

```bash
examples/decomk-selftest/devpod-local/run.sh
```

Explicit action args:

```bash
examples/decomk-selftest/devpod-local/run.sh TUPLE_VERIFY_TOOL TUPLE_VERIFY_CONF TUPLE_CONTEXT_OVERRIDE TUPLE_DEFAULT_SHARED
```

Literal target run:

```bash
examples/decomk-selftest/devpod-local/run.sh all
```

## Context and tuple semantics covered

- Workspace repo name `decomk` is auto-detected as context key.
- `DEFAULT` defines:
  - `TUPLE_VERIFY_TOOL`
  - `TUPLE_VERIFY_CONF`
  - `TUPLE_CONTEXT_OVERRIDE`
  - `TUPLE_DEFAULT_SHARED`
  - `TUPLE_STAMP_PROBE`
  - `TUPLE_STAMP_VERIFY`
- `decomk` context overrides only `TUPLE_CONTEXT_OVERRIDE`.
- Test expectation:
  - context tuple override is applied,
  - other DEFAULT tuples remain available,
  - tool repo origin matches the temporary git server URL,
  - config repo origin matches the temporary git server URL,
  - make runs in `DECOMK_STAMPDIR`,
  - stamp probe target does not rerun once stamped.

## PASS/FAIL markers

`run.sh` requires these log markers from `make.log`:

- `SELFTEST PASS conf-repo-origin`
- `SELFTEST PASS tool-repo-origin`
- `SELFTEST PASS context-override`
- `SELFTEST PASS default-tuple-available`
- `SELFTEST PASS stamp-dir-working-dir`
- `SELFTEST PASS stamp-probe-ran`
- `SELFTEST PASS stamp-idempotent`

Any `SELFTEST FAIL ...` marker is treated as failure.
