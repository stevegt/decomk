# TODO 013 - conf repo init scaffolding (`decomk init-conf`)

## Decision Intent Log

ID: DI-013-20260422-110500
Date: 2026-04-22 11:05:00
Status: active
Decision: Add a first-class `decomk init-conf` command that scaffolds a shared config repo from embedded templates, including a starter `.devcontainer` producer tree, with strict non-overwrite behavior matching `decomk init`.
Intent: Make shared conf repo bootstrap reproducible and low-friction for DevOps while keeping generated starter content plain-English, heavily commented, and aligned with decomk lifecycle/producer contracts.
Constraints: Keep default output at current git repo root, generate fixed placeholders (no prompts), include `bin/hello-world.sh`, and enforce drift sync with generated examples + tests.
Affects: `cmd/decomk/main.go`, `cmd/decomk/init_conf.go`, `cmd/decomk/templates/confrepo.*`, `cmd/decomk/init_conf_test.go`, `cmd/confrepogen/main.go`, `cmd/decomk/confrepo_sync_test.go`, `examples/confrepo/*`, `README.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`, `TODO/TODO.md`.

ID: DI-013-20260422-143000
Date: 2026-04-22 14:30:00
Status: active
Decision: Close TODO 013 by wiring conf-repo generated-file drift checks into `make check-generated` and updating README onboarding/CLI/safety documentation to treat `decomk init-conf` as the primary shared-conf bootstrap path.
Intent: Make `init-conf` operationally complete (command + templates + tests + docs + verification entrypoints) so DevOps users can bootstrap shared conf repos with one documented workflow.
Constraints: Keep conservative overwrite semantics, preserve TODO numbering, and keep template ownership guidance explicit for both `init` and `init-conf`.
Affects: `Makefile`, `README.md`, `TODO/013-conf-repo-init-scaffolding.md`, `TODO/TODO.md`.

## Goal

Provide an opinionated `decomk init-conf` command that creates a runnable
starter shared config repo with:

- `decomk.conf`
- `Makefile`
- `README.md`
- `bin/hello-world.sh`
- `.devcontainer/devcontainer.json`
- `.devcontainer/decomk-stage0.sh`
- `.devcontainer/Dockerfile`

The generated content must help a DevOps engineer quickly understand:

- what a context is,
- what tuples are,
- how phase-aware targets work (`updateContent` vs `postCreate`),
- how to bootstrap a genesis image once, then switch to an `image:` stanza.

## Subtasks

- [x] 013.1 Add new `decomk init-conf` command routing and usage text.
- [x] 013.2 Add embedded confrepo templates and rendering path in `cmd/decomk`.
- [x] 013.3 Implement strict overwrite policy (`-f`/`-force`) for all managed confrepo files.
- [x] 013.4 Generate `.devcontainer` producer scaffolding by default (genesis build + migration comments).
- [x] 013.5 Add confrepo generator (`cmd/confrepogen`) and `go generate` hook.
- [x] 013.6 Add sync test to keep `examples/confrepo/*` in lockstep with templates.
- [x] 013.7 Add command tests for `init-conf` behavior and safety policies.
- [x] 013.8 Update README onboarding to use `decomk init-conf`.
- [x] 013.9 Add/refresh TODO links from TODO 001 and TODO index.

## Acceptance criteria

- `decomk init-conf` scaffolds the full starter tree in an empty git repo.
- Without `-f`, existing managed files cause a hard refusal with reconciliation guidance.
- With `-f`, managed files are updated atomically and status lines are reported.
- Generated `examples/confrepo/*` matches template rendering in tests.
- README onboarding shows command-driven conf repo bootstrap.
