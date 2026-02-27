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

The intended container layout is:

```text
/
├── var
│   └── decomk
│       ├── conf/                 (git clone of shared config repo)
│       │   └── etc/
│       │       ├── decomk.conf
│       │       ├── decomk.d/*.conf   (optional)
│       │       └── Makefile
│       ├── stamps/               (global stamp dir; make working directory)
│       ├── env.sh                (env exports for other processes to source)
│       ├── log/                  (audit logs; per make invocation)
│       ├── conf.lock             (advisory lock used during git pull/clone)
│       ├── decomk.lock           (advisory lock used during tool self-update)
│       └── stamps/.lock          (advisory lock used during stamp mutation)
└── workspaces/
    ├── repo1/                    (WIP repo clone)
    ├── repo2/                    (WIP repo clone)
    ├── decoconf/                 (optional WIP clone of the config repo)
    └── decomk/                   (this repo; tool source)
```

Key idea: decomk configures the *container* based on the set of WIP repos present
under `/workspaces`, but it does not keep state inside those repos.






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
- Self-update model (isconf-style):
  - decomk keeps a canonical clone of its own repo under `<DECOMK_HOME>/decomk`
  - each invocation runs `git pull --ff-only`, rebuilds, and re-execs into the
    updated binary
  - tool repo URL is provided via the CLI (`-tool-repo`) or env (`DECOMK_TOOL_REPO`)
    (with a devcontainer-friendly local inference fallback)
- Config repo model:
  - a shared config repo is cloned/pulled into `<DECOMK_HOME>/conf` (default `/var/decomk/conf`)
  - the repo URL is provided via the CLI (`-conf-repo`) or env (`DECOMK_CONF_REPO`)
- Pilot repo: `mob-sandbox` (devcontainer + `postCreateCommand`).

## High-level overview

`decomk` is a small CLI wrapper around `make`, modeled after isconf’s
split between:
- `etc/rc.isconf` (bootstrap/refresh) and
- `bin/isconf` (resolve context → write env export file → run make)

### Phase A: Bootstrap (rc.isconf analog)

What rc.isconf does (conceptually) is: ensure the tool + its config
tree are present and up to date, establish identity (domain/host),
ensure a stamps directory exists, then hand off to the main “apply”
command.

Like `rc.isconf`, decomk needs a bootstrap/update step, but the current scope is
intentionally small and focused on container-local state:
- ensure `<DECOMK_HOME>` exists (default `/var/decomk`) and required subdirs
  exist (`conf/`, `stamps/`, `log/`)
- clone/pull the decomk tool repo into `<DECOMK_HOME>/decomk`, rebuild, and
  re-exec into the updated binary (self-update)
- optionally clone/pull the shared config repo into `<DECOMK_HOME>/conf` when
  `-conf-repo` / `DECOMK_CONF_REPO` is set
- discover workspace repos (git checkouts) under the workspace parent directory
  (often `/workspaces/*`) so the container can be configured based on multiple
  WIP repos present

For `decomk`, that translates to:
1. Choose a writable persistent home (see “Persistent directories”).
2. Acquire locks for:
   - config repo updates (`<DECOMK_HOME>/conf.lock`)
   - global stamp mutation (`<DECOMK_HOME>/stamps/.lock`)
3. Refresh the configs/makefiles repo (when configured):
   - clone into `<DECOMK_HOME>/conf` if missing
   - `git pull --ff-only` to update (best-effort if offline)
4. Discover workspace repos and resolve a config key for each (when present).
5. Create required state subdirs (stamps/log) as needed.

### Phase B: Plan + apply (isconf analog)

What `bin/isconf` does (conceptually) is: take a context key, expand it
via `expandmacro`, write `etc/environment` via `mk_env`, then run `make`
in the stamps directory.

For `decomk`, that translates to:
1. Choose which context keys to apply:
   - if `-context` / `DECOMK_CONTEXT` is set: apply that single key (must exist)
   - otherwise: scan workspaces and select at most one key per workspace repo,
     preferring `owner/repo` then `repo` then directory basename (only when the key
     exists in config)
   - apply `DEFAULT` first when defined, then the selected per-workspace keys
2. Expand macros recursively into a flat token list (isconf
   `expandmacro` semantics, but with cycle detection and max depth).
3. Partition the expanded tokens into:
   - `NAME=value` tuples (to pass to `make` on argv), and
   - make targets (everything else).
4. Write an env export file (`<DECOMK_HOME>/env.sh`) from the tuples plus computed variables.
5. Run GNU `make` as a subprocess with:
   - working directory = the stamps directory (so stamps live outside
     the repo), and
   - `-f <Makefile>` (explicit path, typically from `<DECOMK_HOME>/conf/Makefile`),
   - argv = tuples + targets.
6. Exit with `make`’s status code, keeping audit logs for post-mortem.

## Persistent directories (config + stamps)

The Dev Container spec does not mandate a single persistent state
directory. For decomk, prefer a **container-local** root directory and
do not default to `$HOME` (because `$HOME` may be shared across multiple
containers in some devcontainer hosts).

Directory selection (in priority order) for decomk's **state root** (config repo
clone, stamps, env exports, and locks):
1. `DECOMK_HOME` (absolute path): if set, use it for state.
2. Default: `/var/decomk`

Notes:
- The chosen directory must be writable by the dev user. For
  `/var/decomk`, the devcontainer image should create it and `chown` it
  appropriately.
- If you explicitly set `DECOMK_HOME` under `$HOME`, always namespace
  state carefully to avoid collisions across multiple containers that share the
  same home volume.

Audit logs are written separately under `/var/log/decomk` (not under
`DECOMK_HOME`).

Proposed layout (mostly under `DECOMK_HOME` or `/var/decomk`):
- Note: this layout assumes the “make-as-engine” direction discussed in
  TODO 003. If we later replace `make` as the execution/state engine,
  the repo and state layout may change.
- Tool repo clone (self-update):
  - `.../decomk/` (git clone)
    - `.../decomk/bin/decomk` (built binary)
- Configs/makefiles repo clone (policy):
  - `.../conf/` (git clone)
    - `.../conf/decomk.conf`
    - `.../conf/decomk.d/*.conf` (optional)
    - `.../conf/Makefile`
- Execution state:
  - `.../stamps/` (global stamp dir; make working directory)
  - `.../env.sh` (env exports for other processes to source)
  - `/var/log/decomk/<runID>/make.log` (audit logs; per make invocation)
  - `.../decomk.lock`, `.../conf.lock`, and `.../stamps/.lock` (advisory locks)

When using `/var/decomk`, use the same internal tree under it (audit logs still
go to `/var/log/decomk`).

### Workspace discovery and context keys

In a typical devcontainer there may be multiple WIP repos under `/workspaces/*`.
decomk scans a configurable workspaces root directory (default `/workspaces`,
override with `-workspaces` / `DECOMK_WORKSPACES_DIR`) and
derives identity hints for each repo:
- `owner/repo` from `remote.origin.url` when possible
- `repo` from origin URL or directory basename
- directory basename

For each discovered repo, decomk selects at most one matching config key (first
match wins, most specific to least specific) and applies:
- `DEFAULT` first when defined, then
- the selected per-workspace keys (deduplicated),
as the seed token list for macro expansion.

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
2. config repo clone (e.g., `<DECOMK_HOME>/conf/decomk.conf` + optional `decomk.d/*.conf`)

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

The “stamps directory” is the working directory where `make` creates stamp files.
For decomk it is global within the container: `<DECOMK_HOME>/stamps`.

Stamps are the make targets themselves (file targets). Make decides what needs
to run based on whether the target file exists and is up to date.

Conventions (to keep this predictable):
- Targets invoked by `decomk` should be **file targets** whose recipes
  create/update `$@` on success (often via `touch $@`).
- `decomk` runs `make` with `Cmd.Dir = <stampDir>`, so `$@` lands in the
  persistent stamps directory (not in the repo).
- Re-running is fast because already-present stamp files are up to date.

Invalidation policy (MVP):
- To re-run one target: delete its stamp file from the stamps directory.
- To re-run everything: delete the entire stamps directory (future:
  `decomk clean`).

Required (isconf-inspired) hardening:
- Before invoking `make`, decomk should `touch` all existing stamp files in the
  stamps directory. This makes stamps an explicit “I want to re-run”
  mechanism (delete stamp), rather than allowing incidental timestamp/prereq
  changes to trigger re-runs.

Global-stamp implication:
- Target/stamp names should be **container-scoped** (e.g., `install-neovim`), not
  repo-scoped, unless you intentionally bake repo identity into the target name.

## Make execution

Run `make` with:
- `Cmd.Dir = stampDir` (the persistent stamps directory)
- `Cmd.Env = os.Environ()` plus any required overrides (minimal)
- Arguments:
  - variable tuples as `NAME=value` argv entries
  - then targets

Pass-through variables (recommended as both env vars and make tuples):
- `DECOMK_HOME=<home>`
- `DECOMK_STAMPDIR=<stampDir>`
- `DECOMK_WORKSPACES=<space-separated workspace basenames>`
- `DECOMK_CONTEXTS=<space-separated context keys used for expansion>`
- `DECOMK_PACKAGES=<space-separated make targets actually invoked>`

Output handling:
- Stream output to terminal by default (good for lifecycle hooks).
- Also tee to an audit log file for post-mortem debugging.

Locking:
- Use a global stamps lock file to prevent concurrent runs from
  overlapping stamp updates (`flock`-style; implement in Go).

## Go package layout (MVP)

Keep packages as small, root-level directories (no `internal/`, no `pkg/`):
- `cmd/decomk/`: main + CLI parsing
- `state/`: resolve config/state directories + locks + stamps helpers
- `contexts/`: load/merge decomk.conf
- `expand/`: macro expansion algorithm + cycle detection
- `makeexec/`: subprocess wrapper around `make`

Prefer the standard library for CLI (`flag`) unless/until subcommands
become painful.

## CLI shape

Proposed commands:
- `decomk plan` (print resolved tuples/targets + env exports; run `make -n`; do not write `env.sh`)
- `decomk run` (default; resolves + stamps + runs `make`)
- `decomk status` (show existing stamp files)
- `decomk clean` (remove stamp files; all or selected)

Common flags:
- `-C <dir>` (starting directory; like `make -C`)
- `-workspaces <dir>` (workspaces root directory to scan; default `/workspaces`)
- `-context <key>` (force context; bypass auto-detect)
- `-config <path>` (explicit config file path)
- `-makefile <path>` (override default `Makefile`)
- (no flag) always pre-touch existing stamp files before running `make`
- `-force` (force rebuild; e.g., `make -B` or run in a fresh stamp dir)
- `-v` (verbose)

## Subtasks

- [x] 002.1 Decide default persistent directory policy (`/var/decomk` by default; `DECOMK_HOME` override) and document it.
- [ ] 002.2 Specify `decomk.conf` grammar + search/merge precedence (config repo vs explicit `-config`).
- [ ] 002.3 Define workspace discovery + context-key selection (git origin + basename fallbacks; merged multi-workspace expansion).
- [ ] 002.4 Specify tokenization/quoting rules (single quotes) and how they map to `exec.Command` argv (no shell).
- [ ] 002.5 Specify macro expansion semantics (isconf-like; add cycle detection + max depth).
- [ ] 002.6 Specify stamp directory conventions (file targets, optional pre-touch, and clean/force behaviors).
- [ ] 002.7 Specify env export file format + stable path (`.../env.sh`).
- [ ] 002.8 Specify audit record format + file set written per run.
- [ ] 002.9 Confirm package layout fits repo conventions (no `internal/`/`pkg/`; minimal deps).
- [ ] 002.10 Specify config repo update behavior (clone/pull, offline mode, and failure policy).
