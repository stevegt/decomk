# TODO 012 - mob-sandbox pilot for decomk stage-0

## Decision Intent Log

ID: DI-012-20260421-221834
Date: 2026-04-21 22:18:34
Status: active
Decision: Use this TODO as the authoritative cross-repo execution checklist for TODO `001.8`, with each task labeled by the repository where it must be performed.
Intent: Make pilot execution decision-complete across `decomk`, `mob-sandbox`, and `decomk-conf-cswg`, and ensure closure of TODO `001.8` is evidence-driven.
Constraints: Keep `decomk` templates/docs/test generalization in this repo; keep pilot configuration/migration work in `mob-sandbox` and `decomk-conf-cswg`; close `001.8` only after Codespaces evidence is captured.
Affects: `TODO/012-mob-sandbox-pilot.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`, `TODO/TODO.md`.

## Goal

Pilot decomk stage-0 lifecycle integration in `mob-sandbox` against the
real shared config repo `decomk-conf-cswg`, then generalize the resulting
failure-policy and documentation contracts in the decomk repo.

## Execution repos

- `decomk` (this repo): templates, selftests, docs, TODO closure.
- `mob-sandbox`: devcontainer lifecycle hook migration and pilot runs.
- `decomk-conf-cswg`: `decomk.conf` + Makefile policy/target definitions.

## Subtasks

- [ ] 012.1 [repo: decomk-conf-cswg] Add/confirm a `mob-sandbox` context key in `decomk.conf`.
- [ ] 012.2 [repo: decomk-conf-cswg] Add/confirm phase-aware make target composition keyed on `DECOMK_STAGE0_PHASE` (`updateContent` vs `postCreate`).
- [ ] 012.3 [repo: decomk-conf-cswg] Preserve GUI-specific behavior using tuple/env passthrough where required.
- [ ] 012.4 [repo: decomk-conf-cswg] Ensure explicit error handling paths (no silent `|| true`).
- [ ] 012.5 [repo: mob-sandbox] Update `.devcontainer/devcontainer.json` (and GUI variant if used) to call `.devcontainer/decomk-stage0.sh updateContent` and `.devcontainer/decomk-stage0.sh postCreate`.
- [ ] 012.6 [repo: mob-sandbox] Set `DECOMK_CONF_URI` to the real `decomk-conf-cswg` URI and set the chosen `DECOMK_TOOL_URI`.
- [ ] 012.7 [repo: mob-sandbox] Set `DECOMK_FAIL_NOBOOT` explicitly for pilot test cases.
- [ ] 012.8 [repo: mob-sandbox] Remove the old active bootstrap logic from the legacy postCreate path.
- [ ] 012.9 [repo: decomk] Generalize stage-0 failure behavior in templates with `DECOMK_FAIL_NOBOOT` (truthy => non-zero failure; default => marker + continue boot).
- [ ] 012.10 [repo: decomk] Write/clear canonical stage-0 failure marker files under `DECOMK_HOME` and add best-effort MOTD hinting with graceful fallback.
- [ ] 012.11 [repo: decomk] Regenerate stage-0 generated files and keep sync tests passing.
- [ ] 012.12 [repo: decomk] Add selftest coverage for both failure-mode contracts (`DECOMK_FAIL_NOBOOT=true` and default continue-boot mode).
- [ ] 012.13 [repo: decomk] Update docs (`README.md`, `doc/decomk-design.md`, and selftest docs) for `DECOMK_FAIL_NOBOOT` + marker behavior.
- [ ] 012.14 [repo: mob-sandbox] Run Codespaces pilot validation and capture lifecycle/log artifacts for success and injected-failure scenarios.
- [ ] 012.15 [repo: decomk] Link pilot evidence in this TODO and then mark TODO `001.8` complete.

## Evidence requirements

Minimum evidence set from the `mob-sandbox` Codespaces pilot:

- lifecycle evidence that both hooks ran (`updateContent`, `postCreate`),
- success-path decomk run evidence,
- injected-failure evidence with `DECOMK_FAIL_NOBOOT` unset/false showing:
  - boot completion,
  - marker creation,
  - visible warning/hint behavior,
- injected-failure evidence with `DECOMK_FAIL_NOBOOT=true` showing non-zero failure behavior,
- final pass/fail summary with artifact paths.

## Completion criteria

TODO `001.8` may be checked only when all are true:

- all `012.*` subtasks are complete,
- Codespaces evidence is linked in this file,
- decomk template/test/doc updates are complete and consistent with the pilot contract.
