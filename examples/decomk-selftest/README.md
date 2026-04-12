# decomk self-test

This directory contains automated self-test harnesses that validate decomk bootstrap behavior end-to-end across local DevPod and GitHub Codespaces.

Template note:
- `workspace-template/.devcontainer/devcontainer.json` and `workspace-template/.devcontainer/postCreateCommand.sh` are generated from `cmd/decomk/templates/*`.
- Regenerate from repo root with `go generate ./...` (or `make generate`).

## Current scope

- Local DevPod with Docker provider (automated)
- GitHub Codespaces parity harness (automated)
- Remote GCP provider checks (deferred)

## Harness model

1. `run.sh` creates a temporary fixture config repo from `fixtures/confrepo/`.
2. `run.sh` also creates a temporary bare tool repo from the current decomk checkout.
3. `run.sh` serves both repos over `git://` via a temporary local `git daemon`.
4. DevPod starts a workspace from `workspace-template/.devcontainer/`.
5. `postCreateCommand.sh` performs stage-0 bootstrap:
   - ensures a `decomk` binary exists in `PATH` (install-first by default; selftest uses clone mode),
   - clone/pull config repo from `DECOMK_CONF_URI`.
6. `postCreateCommand.sh` runs `decomk run`.
7. `run.sh` reads container make logs and enforces PASS/FAIL markers.
8. `run.sh` then runs two explicit stamp regression invocations:
   - `decomk run TUPLE_STAMP_PROBE`
   - `decomk run TUPLE_STAMP_PROBE TUPLE_STAMP_VERIFY`

## Run

From repo root, convenience wrappers are available:

```bash
make selftest-devpod
make selftest-codespaces
make selftest-codespaces-clean
```

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

Codespaces parity run (requires local branch pushed):

```bash
examples/decomk-selftest/codespaces/run.sh
```

Codespaces parity run with explicit action args:

```bash
examples/decomk-selftest/codespaces/run.sh TUPLE_VERIFY_TOOL TUPLE_VERIFY_CONF TUPLE_CONTEXT_OVERRIDE TUPLE_DEFAULT_SHARED
```

Codespaces parity run with explicit config URI override:

```bash
examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/<owner>/<conf-repo>.git
```

Codespaces parity run with explicit machine override:

```bash
examples/decomk-selftest/codespaces/run.sh --machine <machine-name>
```

Codespaces parity run with local artifact cleanup on success:

```bash
examples/decomk-selftest/codespaces/run.sh --cleanup
```

By default, the harness resolves machine type from the repository-allowed
Codespaces machine list (prefers `basicLinux32gb` when available).
By default, local harness artifacts under `/tmp/decomk-codespaces.*` are kept
for post-run inspection.
Artifact collection is explicit: inspect `diagnostics-summary.txt`,
`diag-<step>.rc`, `diag-<step>.stdout.log`, and `diag-<step>.stderr.log`.
Wait for `diagnostics.complete` before assuming artifact capture is finished.

The Codespaces selftest devcontainer enables `ghcr.io/devcontainers/features/sshd:1`
because the harness runs all remote checks through `gh codespace ssh`.

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
