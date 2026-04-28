# TODO 016 - Consumer `decomk init` minimal template + image-source prompt rework

## Decision Intent Log

ID: DI-016-20260427-200729
Date: 2026-04-27 20:07:29
Status: active
Decision: Keep producer-only legacy flags recognized in `decomk init` but treat them as misuse in image-consumer mode: fail with per-flag message `-<flag> is only valid with -conf`, print normal init usage, and report the first offending flag in deterministic order. Keep Go's flat flag list and prepend a detailed mode note using "image producer" / "image consumer" terminology, including that the image producer repo is usually the conf repo.
Intent: Keep CLI behavior explicit and user-friendly without introducing hidden mode-specific flag parsers, while making mode semantics obvious from standard help output.
Constraints:
- No migration guidance text for removed consumer behavior; decomk is pre-production.
- Producer-only misuse checks in consumer mode cover `-conf-uri`, `-tool-uri`, `-home`, `-log-dir`, and `-fail-no-boot`.
- Usage output stays flat (`PrintDefaults`) with a prepended explanatory note (no section renderer).
Affects:
- `cmd/decomk/init.go`
- `cmd/decomk/init_test.go`
- `README.md`
- `TODO/001-decomk-devcontainer-tool-bootstrap.md`
Supersedes:
- DI-016-20260428-024605 (legacy-flag guidance wording and usage-output presentation details only)

ID: DI-016-20260428-024605  
Date: 2026-04-28 02:46:05  
Status: superseded  
Decision: Redesign consumer `decomk init` so generated consumer `.devcontainer/devcontainer.json` contains only `name` and `image`, add consumer-facing `-conf-url` (HTTP URL) for image derivation via producer repo lookup, replace legacy consumer env prompts/flags with hard errors, and implement an explicit interactive source-selection menu for image acquisition.  
Intent: Keep consumer repos minimal and low-churn while still giving developers an easy, authoritative path to discover image tags from a familiar GitHub HTTP repo URL.  
Constraints:  
- Consumer output must omit `containerEnv`, `updateContentCommand`, and `postCreateCommand`.  
- Consumer must continue writing `.devcontainer/decomk-stage0.sh` for compatibility with lifecycle hooks provided by image metadata.  
- `-image` must short-circuit conf URL prompting.  
- `-conf-url` accepts HTTP(S) only (not SSH), with optional `?ref=<branch|tag|sha>`.  
- Producer (`decomk init -conf`) behavior remains separate and unchanged by this TODO unless explicitly listed below.  
- Consumer-mode legacy flags `-conf-uri`, `-tool-uri`, `-home`, `-log-dir`, `-fail-no-boot` must fail as mode misuse with clear per-flag errors.  
- JSONC parser used for image derivation must support full-line and inline `//` comments safely.  
Affects:  
- `cmd/decomk/init.go`  
- `cmd/decomk/init_templates.go`  
- `cmd/decomk/templates/consumer.devcontainer.json.tmpl` (new)  
- `cmd/decomk/init_test.go`  
- `README.md`  
- `TODO/001-decomk-devcontainer-tool-bootstrap.md` (DI append)  
- `TODO/TODO.md`  
- `docs/thought-experiments/TE-20260428-020715-consumer-init-image-source.md` (new)  
- `docs/thought-experiments/TE-20260428-023011-template-architecture.md` (new)  
Supersedes:  
- DI-001-20260423-140628 (consumer template/default behavior portions only)  
- DI-001-20260425-113454 (consumer prompt/default/import portions only)

## Goal

Make consumer `decomk init` decision-light and stable by generating only identity (`name`) plus image selection (`image`), while preserving developer ergonomics for discovering the authoritative image from the producer conf repo.

## Context Captured From Cross-Repo Planning

1. Current consumer init prompts and writes too much (`DECOMK_*` env + lifecycle hooks), creating duplication and drift with image metadata responsibilities.
2. Developers often know a conf repo HTTP URL before they know a precise image tag.
3. Asking for image only is simple internally but creates avoidable friction in onboarding.
4. Deriving image from conf repo is useful, but must preserve manual override and robust failure behavior.
5. Producer config and example/selftest templates are closer to “full stage0” than to the new minimal consumer shape.

## Locked Decisions (Conversation Output)

1. Consumer generated `devcontainer.json` includes only `name` and `image`.
2. Consumer generated `devcontainer.json` omits `containerEnv`.
3. Consumer generated `devcontainer.json` omits `updateContentCommand` and `postCreateCommand`.
4. Consumer init continues writing `.devcontainer/decomk-stage0.sh`.
5. Add `-conf-url` for consumer mode; do not continue consumer use of `-conf-uri`.
6. `-conf-url` is HTTP(S) only; no SSH-style git URL forms.
7. `-conf-url` supports optional `?ref=...`.
8. Image derivation uses `git clone` + parse producer `.devcontainer/devcontainer.json`.
9. Derivation parser must handle full-line and inline `//` comments.
10. If `-image` is passed, do not prompt for conf URL.
11. Interactive flow uses menu: `1) conf repo URL 2) image URI 3) keep existing image URI` (option 3 only when existing image is present, and must display current image).
12. Interactive derivation failure warns and continues to manual image prompt.
13. `-no-prompt` with no `-image` and no `-conf-url`: reuse existing image if present, otherwise fail.
14. `-no-prompt` derivation failure is hard fail.
15. Consumer legacy flags fail as mode misuse errors: `-conf-uri`, `-tool-uri`, `-home`, `-log-dir`, `-fail-no-boot`.
16. Existing consumer `build` stanzas are not preserved; consumer output is image-based minimal contract.
17. Template architecture uses 3-template model: producer full template, consumer minimal template, examples/selftest full template.
18. Keep Go's flat init flag list and prepend a detailed mode note (image producer vs image consumer; producer repo usually equals conf repo).

## Scope

In scope:
1. Consumer init prompt/flag/render behavior in `cmd/decomk/init.go`.
2. New consumer template embedding and render path.
3. Unit tests for new behavior and regressions.
4. README updates for consumer command UX.
5. TE artifact docs and DI append updates.

Out of scope:
1. Producer `init -conf` feature expansion.
2. Stage-0 runtime algorithm changes.
3. Changes to checkpoint/publish workflows.
4. Consumer support for SSH conf repo URLs.

## Implementation Design

### A) CLI and Prompt Contract

1. Add consumer flag:
   - `-conf-url <http(s) repo URL[?ref=...]>`
2. Consumer-mode legacy misuse errors:
   - `-conf-uri`
   - `-tool-uri`
   - `-home`
   - `-log-dir`
   - `-fail-no-boot`
   - report first offending flag in deterministic order, then print normal init usage.
3. Interactive image-source menu:
   - Option 1: conf repo URL
   - Option 2: image URI
   - Option 3: keep existing image URI (only shown if existing image exists)
4. Prompt text must explicitly say conf URL is HTTP URL, not SSH URL.
5. If `-image` is provided, skip source-selection menu and conf-url prompt logic entirely.

### B) Resolution/Precedence Rules

Consumer mode:
1. `-image` wins always.
2. Else if `-conf-url` is present, attempt derivation.
3. Else interactive mode shows source menu.
4. Else in `-no-prompt`, use existing local image if present.
5. Else fail with clear guidance.

Interactive failure behavior:
1. Derivation fails -> print warning with root cause.
2. Fall through to manual image URI prompt.

Non-interactive failure behavior:
1. Derivation fails in `-no-prompt` -> hard fail.
2. Missing all image sources in `-no-prompt` and no existing image -> hard fail.

### C) Conf URL Derivation

1. Parse `-conf-url` as HTTP(S) URL plus optional query params.
2. Support `?ref=<branch|tag|sha>`.
3. Clone to temp path pattern:
   - `/tmp/decomk-init-conf-*/confrepo`
4. Checkout `ref` when provided.
5. Read producer `.devcontainer/devcontainer.json`.
6. Parse JSONC with support for:
   - full-line `// comments`
   - inline `// comments`
   - safety for quoted strings containing `//`
7. Extract `image` field.
8. Require non-empty extracted image; otherwise treat as derivation failure.

### D) Template Architecture

1. Add new consumer-only template:
   - `cmd/decomk/templates/consumer.devcontainer.json.tmpl`
2. Keep producer template unchanged:
   - `cmd/decomk/templates/confrepo.devcontainer.json.tmpl`
3. Keep existing full-stage0 template for examples/selftest paths.

### E) Generated Consumer Output Contract

Generated consumer `.devcontainer/devcontainer.json` must contain:
1. `"name": ...`
2. `"image": ...`

Generated consumer `.devcontainer/devcontainer.json` must not contain:
1. `containerEnv`
2. `updateContentCommand`
3. `postCreateCommand`
4. `remoteUser`
5. `containerUser`
6. `updateRemoteUserUID`

Consumer `.devcontainer/decomk-stage0.sh` remains generated and executable.

## Test Plan

Add/update tests in `cmd/decomk/init_test.go` covering:

1. Consumer minimal JSON shape:
   - asserts only `name` + `image`.
   - asserts omitted env and lifecycle fields.
2. `-image` bypasses conf-url prompting.
3. Interactive menu with existing image:
   - option 3 keeps existing image.
4. Interactive menu without existing image:
   - option 3 hidden.
5. Interactive conf-url derivation success:
   - image extracted correctly.
6. Interactive conf-url derivation failure:
   - warning emitted; manual image path succeeds.
7. `-no-prompt` with `-conf-url` derivation success:
   - succeeds.
8. `-no-prompt` derivation failure:
   - fails.
9. `-no-prompt` with neither `-image` nor `-conf-url`:
   - uses existing image when available.
10. `-no-prompt` with no sources and no existing image:
   - fails with guidance.
11. Consumer legacy flags fail as misuse errors:
   - each of `-conf-uri`, `-tool-uri`, `-home`, `-log-dir`, `-fail-no-boot`.
12. HTTP URL parser:
   - accepts http/https.
   - rejects ssh-style forms.
   - handles `?ref=...`.
13. JSONC parser:
   - full-line comments.
   - inline comments.
   - quoted strings containing `//`.

## Documentation Updates

1. Update `README.md` consumer init docs:
   - new `-conf-url` behavior and HTTP URL requirement.
   - menu behavior and image override semantics.
   - legacy consumer flags now report mode misuse.
   - consumer output is minimal and image-centric.

## Thought Experiments (Required Artifacts)

1. `docs/thought-experiments/TE-20260428-020715-consumer-init-image-source.md`
   - Decision under test: image-source UX in consumer init.
   - Alternatives: derive-only, image-only, derive+override.
   - Surviving alternatives and chosen conclusion.
2. `docs/thought-experiments/TE-20260428-023011-template-architecture.md`
   - Decision under test: 1-template vs 2-template vs 3-template model.
   - Rationale for 3-template lock.

## Runtime Path Touch Matrix (Planned)

1. Pattern ID: `TMP_CONF_CLONE`
   - Root bounds: `/tmp`
   - Example: `/tmp/decomk-init-conf-abc123/confrepo`
   - Actions: create/read/delete
   - Purpose: conf repo clone + image derivation
2. Pattern ID: `TMP_TEST_DIR`
   - Root bounds: `/tmp`
   - Examples: `t.TempDir()` paths
   - Actions: create/read/delete
   - Purpose: unit test fixtures

## Subtasks

- [x] 016.1 Add this TODO to `TODO/TODO.md` in priority order.
- [x] 016.2 Append DI entries to `TODO/001-decomk-devcontainer-tool-bootstrap.md` referencing this TODO.
- [x] 016.3 Add TE doc `TE-20260428-020715-consumer-init-image-source.md`.
- [x] 016.4 Add TE doc `TE-20260428-023011-template-architecture.md`.
- [x] 016.5 Add new consumer template file `cmd/decomk/templates/consumer.devcontainer.json.tmpl`.
- [x] 016.6 Embed consumer template in `cmd/decomk/init_templates.go`.
- [x] 016.7 Implement consumer minimal render path in `cmd/decomk/init.go`.
- [x] 016.8 Add `-conf-url` flag and parser in `cmd/decomk/init.go`.
- [x] 016.9 Add consumer legacy-flag misuse validation in `cmd/decomk/init.go`.
- [x] 016.10 Implement interactive source menu flow in `cmd/decomk/init.go`.
- [x] 016.11 Implement keep-existing-image menu option behavior.
- [x] 016.12 Implement conf-url derivation via clone + ref checkout + parse.
- [x] 016.13 Upgrade JSONC comment handling for derivation parser.
- [x] 016.14 Implement non-interactive precedence/failure behavior exactly per locks.
- [x] 016.15 Ensure `-image` short-circuits conf-url prompting.
- [x] 016.16 Keep consumer stage0 script generation unchanged.
- [x] 016.17 Update consumer init tests in `cmd/decomk/init_test.go`.
- [x] 016.18 Update `README.md` for new consumer UX.
- [x] 016.19 Run `go test ./...`.
- [x] 016.20 Run `errcheck ./...`.
- [x] 016.21 Perform comment-delta audit on touched code files.
- [ ] 016.22 Prepare Decision Matrix + Runtime Path Touch Matrix in final handoff.

## Acceptance Criteria

1. Running consumer `decomk init` generates `.devcontainer/devcontainer.json` with only `name` and `image`.
2. Consumer `decomk init` still writes executable `.devcontainer/decomk-stage0.sh`.
3. Interactive mode provides image-source menu with option 3 only when existing image exists.
4. `-image` suppresses conf-url prompt flow.
5. `-conf-url` supports HTTP(S) plus optional `?ref=...` and derives image correctly.
6. Derivation failures behave as locked: interactive warn+manual, non-interactive fail.
7. Consumer legacy flags fail as mode misuse errors and print init usage.
8. Tests cover all locked behaviors and pass.
9. `errcheck ./...` passes.
10. TE docs and DI updates are present and linked.

## Notes for Implementer Codex Session

1. This TODO is intentionally decision-complete; do not reopen behavior choices unless a hard implementation constraint appears.
2. If a new naming/path/runtime decision appears during implementation, stop and ask before mutating additional surfaces.
3. Keep producer and example/selftest template contracts intact unless explicitly required by this TODO.
