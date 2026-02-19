# TODO 002 - decomk: software architecture

Goal: document a concrete, Go-first architecture for `decomk` that:
- resolves a **context** into Make target groups + `VAR=value` tuples
- writes an auditable snapshot of the resolved environment
- runs `make` as a subprocess
- uses stamps so repeated runs converge quickly and safely
- keeps stamps + config in a reasonable persistent directory inside a devcontainer

This TODO is a design doc. It should stay aligned with `TODO/001-...`
but be specific enough to implement directly.

## Constraints and non-goals

Constraints:
- Implementation language: **Go**.
- Execution model: call GNU `make` as a subprocess; `decomk` does not
  re-implement make evaluation.
- State model: stamps + config must live in a persistent directory in
  the container (Dev Container standard-friendly); if in doubt, keep
  everything under `/var/decomk` per-container.

Non-goals (for MVP):
- Managing system package installs directly (that stays in `make`).
- A daemon/service. `decomk` is a CLI invoked by lifecycle hooks.

## High-level overview

`decomk` is a small CLI wrapper around `make`:
- XXX first, run an algorithm that is is equivalent of ~/lab/isconf2/isconf2i-git/etc/rc.isconf
  - XXX describe what this does
- XXX next, run an algorithm that is the equivalent of ~/lab/isconf2/isconf2i-git/bin/isconf
  - XXX describe what this does

XXX rework everything below here based on the above XXX findings

1. Identify **workspace** (repo root) and **context** (e.g., `DEFAULT`,
   `owner/repo`, `repo`) from env + git metadata.
2. Load context→expansions configuration (plus optional repo-local
   overrides).
3. Expand context macros into a flat list of `VAR=value` tuples and
   make targets (the “plan”).
4. Write an **audit snapshot** of the resolved plan.
6. Run `make` with the resolved tuples + targets.
   - run 'make' in the stamps directory -- see 
   - make Decides which targets need to not be rerun (via stamps).
7. On success, write stamps.

## Persistent directories (config + stamps)

The Dev Container spec does not mandate a single persistent state
directory. In practice, we should follow a “user-writable first”
approach, with an explicit override, and a `/var/decomk` fallback.

Directory selection (in priority order):
1. `DECOMK_HOME` (absolute path): if set, use it for everything.
2. XDG-ish user directories:
   - config: `${XDG_CONFIG_HOME:-$HOME/.config}/decomk`
   - state (stamps/audit/locks): `${XDG_STATE_HOME:-$HOME/.local/state}/decomk`
3. Fallback: `/var/decomk` (config + state live together)

Notes:
- If (2) is used, `decomk` should keep config and state separate.
- If the chosen directory is not writable, fail with a clear message
  explaining how to set `DECOMK_HOME` to a writable path.
- If we standardize on `/var/decomk`, ensure devcontainer images create
  it and chown it to the dev user.

Proposed layout (when config/state are split):
- Config:
  - `.../decomk/config.json` (or `contexts.conf` style; see below)
  - `.../decomk/config.d/*.json` (optional drop-ins)
- State:
  - `.../decomk/audit/<workspaceKey>/<runID>.json`
  - `.../decomk/stamps/<workspaceKey>/<contextKey>/<stampKey>.json`
  - `.../decomk/locks/<workspaceKey>.lock`

When using `/var/decomk`, use the same internal tree under it.

### Workspace key

Stamps should be per-workspace (per repo checkout) to avoid collisions.
Define:
- `workspaceRoot`: `git rev-parse --show-toplevel` (fallback: `PWD`)
- `workspaceKey`: stable-ish identifier:
  - preferred: `GITHUB_REPOSITORY` (if present) + `workspaceRoot` basename
  - fallback: hash of `workspaceRoot` absolute path

The key should be filesystem-safe (hex-encoded hash is easiest).

## Configuration model

We need a simple, auditable configuration format that maps:
- context → expansions
- macro → macro (recursion)
- output tokens → (`VAR=value` or `makeTarget`)

MVP recommendation: a small JSON config, because Go can parse it with
the standard library and it’s easy to diff/review.

Example structure (sketch):
- `macros`: map[string][]string
- `contexts`: map[string][]string

Where token strings are either:
- `NAME=value` (treated as a `make` command-line variable assignment)
- `target` (treated as a make target)
- `${MACRO}` (macro expansion token)

Key rules:
- Expansion is deterministic and bounded (cycle detection + max depth).
- Unknown macros are errors (with a helpful “did you mean” list).
- Context resolution order is explicit (e.g., `owner/repo` → `repo` → `DEFAULT`).

Repo-local override (optional but useful for portability):
- If `<repoRoot>/.decomk/config.json` exists, merge it over the global config.
  (Merging rules must be simple: replace arrays by key; no deep magic.)

## Stamp model

A stamp records that a resolved action ran successfully under a specific
input configuration. Stamps must be safe: if inputs change, stamps
should be invalidated automatically.

Define a stamp key as a hash of:
- contextKey
- resolved tuples (sorted)
- resolved target
- decomk version
- config digest (hash of effective config file(s))

Stamp file contents (JSON):
- `stampKey`
- `contextKey`, `workspaceKey`
- `target`
- `tuples` (sorted list)
- `configDigest`
- `startedAt`, `finishedAt`, `duration`
- `makeExitCode`
- optional: `stdoutPath`/`stderrPath` for captured logs

Policy:
- Only write a success stamp on exit code 0.
- On failure, keep the previous stamp (if any) and write an audit record.

## Make execution

Run `make` with:
- `Cmd.Dir = workspaceRoot`
- `Cmd.Env = os.Environ()` plus any required overrides (minimal)
- Arguments:
  - variable tuples as `NAME=value` argv entries
  - then targets

Output handling:
- Stream output to terminal by default (good for lifecycle hooks).
- Also tee to an audit log file for post-mortem debugging.

Locking:
- Use a per-workspace lock file to prevent concurrent runs from
  overlapping stamp updates (`flock`-style; implement in Go).

## Go package layout (MVP)

Keep packages as small, root-level directories (no `internal/`, no `pkg/`):
- `cmd/decomk/`: main + CLI parsing
- `paths/`: resolve config/state directories + workspaceKey
- `config/`: load/merge config + compute configDigest
- `expand/`: macro expansion algorithm + cycle detection
- `audit/`: write audit records + output tee
- `stamp/`: stamp read/write + stampKey computation
- `makeexec/`: subprocess wrapper around `make`

Prefer the standard library for CLI (`flag`) unless/until subcommands
become painful.

## CLI shape

Proposed commands:
- `decomk plan` (print resolved tuples/targets; no `make`)
- `decomk run` (default; resolves + stamps + runs `make`)
- `decomk status` (show stamps for current workspace/context)
- `decomk clean` (remove stamps for current workspace/context)

Common flags:
- `-C <dir>` (workspace root override; like `make -C`)
- `-context <key>` (force context; bypass auto-detect)
- `-config <path>` (explicit config file path)
- `-no-stamp` (always run targets)
- `-v` (verbose)

## Subtasks

- [ ] 002.1 Decide default persistent directory policy (XDG-first vs `/var/decomk`-first) and document it.
- [ ] 002.2 Pick config format/name and write a minimal schema (JSON vs `contexts.conf`-style).
- [ ] 002.3 Define workspaceKey + contextKey algorithms (env + git fallback order).
- [ ] 002.4 Specify macro expansion semantics (cycle detection, max depth, error messages).
- [ ] 002.5 Specify stampKey inputs and stamp invalidation rules.
- [ ] 002.6 Specify audit record format and where it is written.
- [ ] 002.7 Confirm package layout fits repo conventions (no `internal/`/`pkg/`; minimal deps).
