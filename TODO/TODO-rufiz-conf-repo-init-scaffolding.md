# TODO-rufiz: conf repo init scaffolding (`decomk init -conf`)

## Decision Intent Log

ID: DI-migil
Date: 2026-04-24 19:05:04
Status: active
Decision: Replace standalone `decomk init-conf` with `decomk init -conf`, and keep conf-repo producer scaffolding behavior unchanged except for explicit dev user/UID prompting and metadata output used by consumer init.
Intent: Collapse bootstrap UX to one init command while preserving the completed producer scaffold capabilities from TODO-rufiz.
Constraints: No backward-compat alias for `init-conf`; docs/tests/help text must move to `init -conf`; generated producer `.devcontainer/devcontainer.json` remains the authoritative identity source for consumer init.
Affects: `cmd/decomk/main.go`, `cmd/decomk/init.go`, `cmd/decomk/init_conf.go`, `cmd/decomk/*_test.go`, `README.md`, `TODO/TODO-rufiz-conf-repo-init-scaffolding.md`, `TODO/TODO.md`.
Supersedes: DI-hikil (command surface only), DI-hogin (onboarding command wording only)

ID: DI-hikil
Date: 2026-04-22 11:05:00
Status: active
Decision: Add a first-class `decomk init-conf` command that scaffolds a shared config repo from embedded templates, including a starter `.devcontainer` producer tree, with strict non-overwrite behavior matching `decomk init`.
Intent: Make shared conf repo bootstrap reproducible and low-friction for DevOps while keeping generated starter content plain-English, heavily commented, and aligned with decomk lifecycle/producer contracts.
Constraints: Keep default output at current git repo root, generate fixed placeholders (no prompts), include `bin/hello-world.sh`, and enforce drift sync with generated examples + tests.
Affects: `cmd/decomk/main.go`, `cmd/decomk/init_conf.go`, `cmd/decomk/templates/confrepo.*`, `cmd/decomk/init_conf_test.go`, `cmd/confrepogen/main.go`, `cmd/decomk/confrepo_sync_test.go`, `examples/confrepo/*`, `README.md`, `TODO/TODO-jirin-decomk-devcontainer-tool-bootstrap.md`, `TODO/TODO.md`.

ID: DI-hogin
Date: 2026-04-22 14:30:00
Status: active
Decision: Close TODO-rufiz by wiring conf-repo generated-file drift checks into `make check-generated` and updating README onboarding/CLI/safety documentation to treat `decomk init-conf` as the primary shared-conf bootstrap path.
Intent: Make `init-conf` operationally complete (command + templates + tests + docs + verification entrypoints) so DevOps users can bootstrap shared conf repos with one documented workflow.
Constraints: Keep conservative overwrite semantics, preserve TODO handles, and keep template ownership guidance explicit for both `init` and `init-conf`.
Affects: `Makefile`, `README.md`, `TODO/TODO-rufiz-conf-repo-init-scaffolding.md`, `TODO/TODO.md`.

## Goal

Provide an opinionated `decomk init -conf` mode that creates a runnable
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

- [x] rufiz.1 Add new `decomk init -conf` command routing and usage text.
- [x] rufiz.2 Add embedded confrepo templates and rendering path in `cmd/decomk`.
- [x] rufiz.3 Implement strict overwrite policy (`-f`/`-force`) for all managed confrepo files.
- [x] rufiz.4 Generate `.devcontainer` producer scaffolding by default (genesis build + migration comments).
- [x] rufiz.5 Add confrepo generator (`cmd/confrepogen`) and `go generate` hook.
- [x] rufiz.6 Add sync test to keep `examples/confrepo/*` in lockstep with templates.
- [x] rufiz.7 Add command tests for `init -conf` behavior and safety policies.
- [x] rufiz.8 Update README onboarding to use `decomk init -conf`.
- [x] rufiz.9 Add/refresh TODO links from TODO-jirin and TODO index.

## Acceptance criteria

- `decomk init -conf` scaffolds the full starter tree in an empty git repo.
- Without `-f`, existing managed files cause a hard refusal with reconciliation guidance.
- With `-f`, managed files are updated atomically and status lines are reported.
- Generated `examples/confrepo/*` matches template rendering in tests.
- README onboarding shows command-driven conf repo bootstrap.
