# TODO 002 - decomk: software architecture

Goal: document a concrete, Go-first architecture for `decomk` that:
- resolves a **context** into Make target groups + `VAR=value` tuples
- writes an auditable snapshot of the resolved environment
- runs `make` as a subprocess
- uses stamps so repeated runs converge quickly and safely
- keeps stamps + config in a reasonable persistent directory inside a devcontainer

This TODO is a design doc. It should stay aligned with `TODO/001-...`
but be specific enough to implement directly.

## Structure

container/
└── workspaces/
      ├── repo1/  (clone of repo under managment)
      ├── decoconf/  (clone of config repo)




8 directories, 38 files
      

      ├──   
      └── 

decomk/  (tool repo; git clone)



- decomk is this repo, and is common across all installations
- decoconf is this 






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

## Current decisions (tentative)

- State engine: GNU `make` + stamps.
- Stamp behavior: `decomk` pre-touches existing stamps by default (no flag planned).
- Update model: method (B) + self-update is required:
  - `decomk` and its config live under a persistent root (default `/var/decomk`) as git clones.
  - Each run does `git pull --ff-only` and re-execs into the updated tool binary when needed.
- Pilot repo: `mob-sandbox` (devcontainer + `postCreateCommand`).

## High-level overview

`decomk` is a small CLI wrapper around `make`, modeled after isconf’s
split between:
- `etc/rc.isconf` (bootstrap/refresh) and
- `bin/isconf` (resolve context → write env snapshot → run make)

### Phase A: Bootstrap (rc.isconf analog)

What rc.isconf does (conceptually) is: ensure the tool + its config
tree are present and up to date, establish identity (domain/host),
ensure a stamps directory exists, then hand off to the main “apply”
command.

Like `rc.isconf`, `decomk` needs a bootstrap/update step. The intended
model is:
- `decomk` source is a git clone under the persistent decomk root.
- `decomk` configuration + makefiles are a separate git clone under the
  same root.
- Each invocation refreshes both clones with `git pull` (unless
  explicitly disabled), then runs the requested context.

This keeps the workspace repo clean (no generated state), and keeps the
“policy” repo (decomk.conf + make targets) centralized and updateable.
Like lunamake, this policy repo can use a branch-per-environment workflow
(e.g., test → prod) to roll forward tool changes safely.

For `decomk`, that translates to:
1. Choose a writable persistent home (see “Persistent directories”).
2. Acquire a lock for updates and state writes (to prevent concurrent
   `git pull` and stamp mutation).
3. Refresh the `decomk` tool repo:
   - clone if missing
   - `git pull --ff-only` to update
   - if the tool repo changed, rebuild `decomk` and re-exec so the
     running binary matches HEAD
4. Refresh the configs/makefiles repo:
   - clone if missing
   - checkout the configured ref/branch/tag/SHA (default branch, plus
     an optional override) before pulling
   - `git pull --ff-only` to update
5. Identify the **workspace** (repo root) and compute `workspaceKey`.
   - “Workspace” is the repo checkout that drives context selection
     (usually the devcontainer workspace folder). The Makefile and
     context definitions live in the config repo.
   - Installed tools are usually *container/user-scoped* (e.g.
     `$HOME/.local/bin`, `$HOME/.cache`, `/usr/local`) and are managed
     by make recipes. `decomk` can pass hints like `DECOMK_PREFIX` but
     should not try to own tool installation policy itself.
6. Resolve a **contextKey** (e.g., `owner/repo`, `repo`, `DEFAULT`),
   from env + flags.
7. Load configuration and write
   an “effective config” snapshot into state for audit/debugging.
8. Create required state subdirs (stamps/audit/env/locks).

### Phase B: Plan + apply (isconf analog)

What `bin/isconf` does (conceptually) is: take a context key, expand it
via `expandmacro`, write `etc/environment` via `mk_env`, then run `make`
in the stamps directory.

For `decomk`, that translates to:
1. Seed expansion with `DEFAULT` + `contextKey` (and optionally
   `owner/repo` → `repo` fallback if the more specific key is missing).
2. Expand macros recursively into a flat token list (isconf
   `expandmacro` semantics, but with cycle detection and max depth).
3. Partition the expanded tokens into:
   - `NAME=value` tuples (to pass to `make` on argv), and
   - make targets (everything else).
4. Write an env snapshot file (mk_env analog) from the tuples.
5. Run GNU `make` as a subprocess with:
   - working directory = the stamps directory (so stamps live outside
     the repo), and
   - `-f <configRepoRoot>/<makefile>` (explicit path),
   - argv = tuples + targets.
6. Exit with `make`’s status code, keeping audit logs for post-mortem.

## Persistent directories (config + stamps)

The Dev Container spec does not mandate a single persistent state
directory. For decomk, prefer a **container-local** root directory and
do not default to `$HOME` (because `$HOME` may be shared across multiple
containers in some devcontainer hosts).

Directory selection (in priority order):
1. `DECOMK_HOME` (absolute path): if set, use it for everything.
2. Default: `/var/decomk`

Notes:
- The chosen directory must be writable by the dev user. For
  `/var/decomk`, the devcontainer image should create it and `chown` it
  appropriately.
- If you explicitly set `DECOMK_HOME` under `$HOME`, always namespace
  state by `workspaceKey` to avoid collisions across workspaces and/or
  containers that share the same home volume.

Proposed layout (all under `DECOMK_HOME` or `/var/decomk`):
- Note: this layout assumes the “make-as-engine” direction discussed in
  TODO 003. If we later replace `make` as the execution/state engine,
  the repo and state layout may change.
- Tool repo:
  - `.../repos/decomk/` (git clone)
- Configs/makefiles repo:
  - `.../repos/decomk-config/` (git clone)
- State:
  - `.../state/audit/<workspaceKey>/<runID>/plan.json`
  - `.../state/audit/<workspaceKey>/<runID>/make.log`
  - `.../state/env/<workspaceKey>/<contextKey>.sh`
  - `.../state/stamps/<workspaceKey>/<contextKey>/` (make working dir)
  - `.../state/locks/<workspaceKey>.lock`
  - `.../state/locks/update.lock`

When using `/var/decomk`, use the same internal tree under it.

Definitions:
- `toolRepoRoot = <decomkRoot>/repos/decomk`
- `configRepoRoot = <decomkRoot>/repos/decomk-config`

### Workspace key

Stamps should be per-workspace (per repo checkout) to avoid collisions.
Define:
- `workspaceRoot`: `git rev-parse --show-toplevel` (fallback: `PWD`)
- `workspaceKey`: stable-ish identifier:
  - preferred: `GITHUB_REPOSITORY` (if present) + `workspaceRoot` basename
  - fallback: hash of `workspaceRoot` absolute path

The key should be filesystem-safe (hex-encoded hash is easiest).

## Configuration model

We want isconf-like “macros expand to tuples + targets” in a format that
humans can edit and that a Go CLI can parse deterministically.

MVP recommendation: a `decomk.conf` file with the same core semantics
as isconf `conf/hosts.conf`:
- Lines of the form `key: token token token`
- Continuation lines append more tokens to the previous `key:`
- `#` starts a comment (whole-line comments only, for MVP)

Tokens are whitespace-separated shell-words with a small, explicit
quoting rule set:
- single quotes may be used to include spaces in a token (e.g.,
  `FOO='bar baz'`)
- `decomk` must remove quotes while parsing because it will exec `make`
  directly (no shell `eval` step)

Semantics:
- Any `key` can be referenced as a macro token.
- Expansion output tokens are either:
  - `NAME=value` tuples (passed to `make` as command-line variable assignments), or
  - make targets (anything else).

Config precedence (highest wins):
1. `-config <path>` (or `DECOMK_CONFIG`) if provided
2. (optional) repo-local config (e.g., `<repoRoot>/decomk.conf`) for experimentation/overrides
3. config repo (e.g., `<configRepoRoot>/decomk.conf` + optional `decomk.d/*.conf`)

Merging rule (simple and auditable):
- Configs are key→[]token maps; when the same key exists in multiple
  sources, the highest-precedence definition replaces lower ones.

Key rules:
- Expansion is deterministic and bounded (cycle detection + max depth).
- Unknown macros are treated as literals (isconf behavior) unless we
  enable a strict mode.
- Context resolution order is explicit (e.g., `owner/repo` → `repo` → `DEFAULT`).

## Stamp model

`decomk` relies on `make` for stamps, the same way isconf does.

The “stamps directory” is simply the working directory where `make`
creates stamp files. Stamps are the make targets themselves (file
targets), and make decides what needs to run based on whether the
target file exists and is up to date.

Conventions (to keep this predictable):
- Targets invoked by `decomk` should be **file targets** whose recipes
  create/update `$@` on success (often via `touch $@`).
- `decomk` runs `make` with `Cmd.Dir = <stampDir>`, so `$@` lands in the
  persistent stamps directory (not in the repo).
- Re-running is fast because already-present stamp files are up to date.

Invalidation policy (MVP):
- To re-run one target: delete its stamp file from the stamps directory.
- To re-run everything: `decomk clean` removes the stamps directory for
  the workspace/context (or calls an equivalent `make clean`).

Required (isconf-inspired) hardening:
- Before invoking `make`, `decomk` should `touch` all existing stamp files
  in the stamps directory. This makes stamps an explicit “I want to
  re-run” mechanism (delete stamp), rather than allowing incidental
  timestamp/prereq changes to trigger re-runs.

## Make execution

Run `make` with:
- `Cmd.Dir = stampDir` (the persistent stamps directory)
- `Cmd.Env = os.Environ()` plus any required overrides (minimal)
- Arguments:
  - variable tuples as `NAME=value` argv entries
  - then targets

Pass-through variables (recommended as both env vars and make tuples):
- `DECOMK_WORKSPACE_ROOT=<workspaceRoot>`
- `DECOMK_STAMPDIR=<stampDir>`

Output handling:
- Stream output to terminal by default (good for lifecycle hooks).
- Also tee to an audit log file for post-mortem debugging.

Locking:
- Use a per-workspace lock file to prevent concurrent runs from
  overlapping stamp updates (`flock`-style; implement in Go).

## Go package layout (MVP)

Keep packages as small, root-level directories (no `internal/`, no `pkg/`):
- `cmd/decomk/`: main + CLI parsing
- `state/`: resolve config/state directories + workspaceKey
- `contexts/`: load/merge decomk.conf
- `expand/`: macro expansion algorithm + cycle detection
- `audit/`: write audit records + output tee
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
- `-makefile <path>` (override default `Makefile`)
- (no flag) always pre-touch existing stamp files before running `make`
- `-force` (force rebuild; e.g., `make -B` or run in a fresh stamp dir)
- `-v` (verbose)

## Subtasks

- [x] 002.1 Decide default persistent directory policy (`/var/decomk` by default; `DECOMK_HOME` override) and document it.
- [ ] 002.2 Specify `decomk.conf` grammar + search/merge precedence (config repo vs repo-local vs explicit `-config`).
- [ ] 002.3 Define workspaceKey + contextKey algorithms (env + git fallback order + filesystem-safe encoding).
- [ ] 002.4 Specify tokenization/quoting rules (single quotes) and how they map to `exec.Command` argv (no shell).
- [ ] 002.5 Specify macro expansion semantics (isconf-like; add cycle detection + max depth).
- [ ] 002.6 Specify stamp directory conventions (file targets, optional pre-touch, and clean/force behaviors).
- [ ] 002.7 Specify env snapshot format + stable path (`.../env/<workspaceKey>/<contextKey>.sh`).
- [ ] 002.8 Specify audit record format + file set written per run.
- [ ] 002.9 Confirm package layout fits repo conventions (no `internal/`/`pkg/`; minimal deps).
- [ ] 002.10 Specify update behavior (clone/pull, rebuild+re-exec, offline mode, and failure policy).
