# TODO 008 - decomk: no silent failures + explicit error handling

## Decision Intent Log

ID: DI-008-20260412-122157
Date: 2026-04-12 12:21:57
Status: active
Decision: Ban silent failure patterns in decomk scripts and Go code by removing `|| true`, removing explicit ignored Go errors (`_ = ...`), and adding automated verify gates.
Intent: Make failures diagnosable and deterministic by forcing explicit command/error handling and preventing regressions through CI/local verification checks.
Constraints: Preserve existing selftest PASS/FAIL semantics (cleanup/diagnostic capture may log failures without flipping a successful test verdict), keep comment provenance requirements, and avoid hidden fallbacks.
Affects: `Makefile`, `examples/decomk-selftest/codespaces/run.sh`, `examples/decomk-selftest/devpod-local/*.sh`, `examples/decomk-selftest/fixtures/confrepo/scripts/*.sh`, `examples/devcontainer/postCreateCommand.sh`, `cmd/decomk/templates/postCreateCommand.sh.tmpl`, `state/state.go`, `stage0/stage0.go`, `cmd/decomk/main.go`, `cmd/decomk/*_test.go`, `README.md`, `examples/decomk-selftest/README.md`, `AGENTS.md`, `/home/stevegt/.codex/AGENTS.md`.

## Goal

Ensure decomk never hides command or error failures behind silent control-flow shortcuts, and enforce that policy automatically during `make verify`.

## Subtasks

- [x] 008.1 Remove all `|| true` usages from tracked scripts/templates and replace them with explicit return-code handling.
- [x] 008.2 Add explicit diagnostics status artifacts for Codespaces harness cleanup (`diagnostics-summary`, per-step rc/stdout/stderr, completion marker).
- [x] 008.3 Remove explicit ignored Go errors (`_ = ...`) and return/report combined errors where needed.
- [x] 008.4 Add verification gates for shell swallow patterns, Go blank-identifier error ignores, and `errcheck`.
- [x] 008.5 Update repo and global AGENTS policy text to codify the no-silent-failure requirement.
- [x] 008.6 Update user-facing docs for new diagnostics artifact semantics.
