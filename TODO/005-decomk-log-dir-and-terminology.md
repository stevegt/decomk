# TODO 005 - decomk: log dir override + naming cleanup

Goal: make decomk’s per-run `make` output logging robust (no hard dependency on
`/var/log/decomk` being writable) while keeping `/var/log/decomk` as the preferred
default location, and remove confusing “audit” terminology by consistently using
“log”.

## Background / problem

Recent changes moved per-run logs from `<DECOMK_HOME>/log/<runID>/` to
`/var/log/decomk/<runID>/`.

That change has two problems:

1. **Reliability regression**: `decomk run` logs by default, so it now fails with
   `permission denied` in common non-root/container setups where `/var/log/decomk`
   is not pre-created and writable.
2. **Terminology drift**: the code uses “audit” and “log” interchangeably
   (`createAuditDir`, `state.AuditDir`, `mode.Audit`, `make.log`, `LogDir`), which
   makes the intent hard to follow and makes future changes riskier.

Docs also need cleanup:
- README still shows `<DECOMK_HOME>/log` in the example filesystem tree, and its
  algorithm section has tab-indented list items that can render as code blocks.

## Decisions (make these true in code + docs)

### Naming

Use **log** consistently:
- “per-run log dir”
- “run log file”
- avoid “audit” entirely in identifiers, comments, and docs.

### Log root selection (override + fallback)

Add a user-facing override for the per-run log root directory:

Precedence (first match wins):
1. `-log-dir <abs-path>` (new flag; absolute path required)
2. `DECOMK_LOG_DIR` (new env var; absolute path required)
3. default: `/var/log/decomk`

Fallback behavior:
- If the selected log root is the **default** `/var/log/decomk` and decomk cannot
  create the per-run directory (permission denied, etc.), fall back to
  `<DECOMK_HOME>/log`.
- If the log root is **explicit** (`-log-dir` or `DECOMK_LOG_DIR`) and decomk
  cannot create the per-run directory, fail with an error (do not silently
  redirect).

Rationale:
- Default keeps logs in the conventional system log location.
- Fallback keeps `decomk run` working in environments where `/var/log` is not
  writable, without requiring users to pre-provision anything.
- Explicit settings should be strict so misconfiguration is obvious.

## Implementation notes (decision-complete)

1. **Flags/config**
   - Add `commonFlags.logDir string`.
   - Add `-log-dir` in `addCommonFlags`:
     - help text: “per-run log root directory (overrides DECOMK_LOG_DIR; default /var/log/decomk)”
     - require absolute paths (mirror `state.Home`’s behavior: reject relative).
   - Add env var `DECOMK_LOG_DIR`.

2. **Plan plumbing**
   - Extend `resolvedPlan` with:
     - `LogRoot string`
     - `LogRootExplicit bool` (true when set via flag/env)
   - Resolve `LogRoot` in `resolvePlanFromFlags` so downstream code has a single
     source of truth.

3. **Dir creation + naming cleanup**
   - Rename:
     - `executionMode.Audit` → `executionMode.Log`
     - `createAuditDir` → `createRunLogDir`
     - local vars `auditDir` → `runLogDir`, `logPath` → `runLogPath`
   - Stop routing through `state.AuditDir(home, runID)` since log root is not
     necessarily derived from `home`.
   - Implement `createRunLogDir(plan *resolvedPlan, runID string, stderr io.Writer) (string, error)`:
     - first attempt: `filepath.Join(plan.LogRoot, runID)`
     - if that fails and `!plan.LogRootExplicit`:
       - retry under `filepath.Join(plan.Home, "log", runID)`
       - print a one-line warning to `stderr` describing the fallback and how to
         override (`-log-dir` / `DECOMK_LOG_DIR`)
     - if that fails (or if explicit), return an error that includes the path
       attempted.
   - Keep the “unique dir” behavior (suffix `-2`, `-3`, …) to avoid collisions.

4. **State package**
   - Remove/rename `AuditDir` (no “audit” terminology).
   - Ensure `state.LogDir(home)` is the **home-based** log dir (`<home>/log`).
   - Keep a constant for the preferred default log root:
     - `state.DefaultLogDir = "/var/log/decomk"` (used by cmd for default choice).

5. **Tests**
   - Add table-driven tests for log-root selection precedence:
     - flag beats env beats default
     - relative paths are rejected
   - Add a fallback test that forces a log-root mkdir failure:
     - use a temp dir with permissions that prevent creating a child dir
     - verify fallback to `<home>/log/<runID>` happens only when default/root-not-explicit
     - verify explicit log root failure returns an error (no fallback)

6. **Docs**
   - `README.md`:
     - update filesystem tree to show `/var/log/decomk/<runID>/make.log`
       (and remove `<DECOMK_HOME>/log` from the example)
     - add a short note: ensure `/var/log/decomk` is writable **or** set `-log-dir`
       / `DECOMK_LOG_DIR`; mention fallback to `<DECOMK_HOME>/log` when using the
       default and it is not writable
     - replace tab-indented bullets in algorithm section with spaces so Markdown
       nesting renders correctly
   - `TODO/002-decomk-architecture.md`:
     - replace “audit” wording with “log”
     - document the same default/override/fallback policy as README

## Subtasks
- [x] 005.1 Rename “audit” identifiers/comments to “log” (cmd + state + docs).
- [x] 005.2 Add `-log-dir` flag and `DECOMK_LOG_DIR` env var (absolute-only).
- [x] 005.3 Resolve/store `LogRoot` + `LogRootExplicit` in `resolvedPlan`.
- [x] 005.4 Implement per-run log dir creation with default-only fallback to `<home>/log`.
- [x] 005.5 Remove/replace `state.AuditDir`; restore `state.LogDir(home)` to `<home>/log`; keep `state.DefaultLogDir`.
- [x] 005.6 Add unit tests for precedence + fallback behavior.
- [x] 005.7 Update `README.md` and `TODO/002-decomk-architecture.md` for consistency + fix tab-indented bullets.
- [x] 005.8 Run `gofmt` + `go test ./...`.
