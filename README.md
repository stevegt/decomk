# decomk

`decomk` is an isconf-inspired bootstrap wrapper for devcontainers.

It resolves a **context** (e.g., `owner/repo`, `repo`, `DEFAULT`) into:
- a list of `NAME=value` **tuples** to pass to `make`
- a (possibly empty) default list of **make targets** from config tokens
- a generated env export file (`<DECOMK_HOME>/env.sh`) for other processes to source

It then selects the final make targets from positional **action args**
(isconf-style) and runs **GNU make** as a subprocess in a persistent **stamp
directory**, so repeated runs converge quickly.

If you want background on isconf-style bootstraps, see https://infrastructures.org/.

## Status

This repo contains an MVP implementation in Go:
- `decomk init`: scaffold `.devcontainer/devcontainer.json` + `.devcontainer/postCreateCommand.sh` from embedded templates
- `decomk plan`: resolve + print the plan; print env exports (dry-run; does **not** write `<DECOMK_HOME>/env.sh`); run `make -n` in the stamp dir
- `decomk run`: resolve + write env export file; run `make` in the stamp dir
- `go generate ./...` (or `make generate`): regenerate canonical example scaffold files from those same templates

Planned work lives under `TODO/`.

Design notes:
- `doc/isconf-design.md` describes the reverse-engineered isconf algorithm and rationale.
- `doc/decomk-design.md` describes decomk selftest design decisions and selector semantics.

## Quick start (MVP)

For a new repo, scaffold the lifecycle files first:

```bash
go run ./cmd/decomk init -conf-uri git:<git-url>
```

Regenerate canonical example scaffold files when templates change:

```bash
make generate
make check-generated
```

1) Create `decomk.conf` and a `Makefile`.

For experimentation, you can put both in your workspace repo root and pass them
explicitly with `-config` and `-makefile` (this avoids depending on any config
repo clone).

For a typical devcontainer setup, put `DECOMK_CONF_URI` (and optionally
`DECOMK_TOOL_URI`) in `.devcontainer/devcontainer.json`, and run the reference
`.devcontainer/postCreateCommand.sh` lifecycle hook. That hook performs stage-0
bootstrap (ensures `decomk` is in `PATH`, syncs config repo from URI, then runs decomk).
The checked-in `examples/devcontainer/devcontainer.json` is standalone and
includes a local Dockerfile build definition.

DECOMK_HOME defaults to /var/decomk, so the config repo clone lives under `/var/decomk/conf`. 

By default, decomk writes per-run logs under `/var/log/decomk` (override with
`-log-dir` / `DECOMK_LOG_DIR`).

`decomk.conf`:
```conf
# Context definitions (macros).
DEFAULT: Block00_base Block10_common FOO='bar baz'

# Repo-specific composition (context key).
myrepo: DEFAULT Block20_go
```

`Makefile`:
```make
SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c

# IMPORTANT: decomk runs make in the stamp directory.
# That means $@ is a file path under the stamp dir, and touching $@
# records "this target succeeded".

Block00_base:
	echo "base tools"
	touch $@

Block10_common: Block00_base
	echo "common tools (FOO=$(FOO))"
	touch $@

Block20_go: Block10_common
	echo "install go tools"
	touch $@
```

2) Run `plan`:
```bash
DECOMK_HOME=/tmp/decomk go run ./cmd/decomk plan -config ./decomk.conf -makefile ./Makefile -context myrepo
```

3) Run `make` via `decomk`:
```bash
DECOMK_HOME=/tmp/decomk DECOMK_LOG_DIR=/tmp/decomk/log go run ./cmd/decomk run -config ./decomk.conf -makefile ./Makefile -context myrepo
```

To install a binary instead of using `go run`:
```bash
go install ./cmd/decomk
```

## Worked example: 

Example container filesystem tree:
  - WIP repos are under `/workspaces/*`
  - decomk keeps persistent state under `/var/decomk/*`
  - decomk writes per-run logs under `/var/log/decomk/*` by default
  - the shared config repo is cloned under `/var/decomk/conf` (not under `/workspaces`)
  - when `DECOMK_TOOL_URI` uses `git:...`, the tool repo is cloned under `/var/decomk/src/decomk`

```text
/
├── var
│   ├── decomk
│   │   ├── src
│   │   │   └── decomk             (tool repo clone; clone mode only)
│   │   ├── conf                   (config repo clone; managed by stage-0 hook)
│   │   │   ├── decomk.conf        (configuration file for all managed repos)
│   │   │   ├── decomk.d
│   │   │   │   └── *.conf
│   │   │   └── Makefile           (Makefile for all managed repos)
│   │   ├── env.sh                 (env exports for other processes to source)
│   │   ├── stamps
│   │   │   ├── install-codex      (example)
│   │   │   ├── install-mob-consensus (example)
│   │   │   └── install-neovim     (example)
│   └── log
│       └── decomk
│           └── <runID>
│               └── make.log        (per-run make output)
└── workspaces
    ├── repo1  (example)
    └── repo2  (example)

```

If `/var/log/decomk` is not writable and you did not explicitly set
`-log-dir`/`DECOMK_LOG_DIR`, decomk falls back to `<DECOMK_HOME>/log`.

Example `/var/decomk/conf/decomk.conf`:
```conf
DEFAULT: Block00_base Block10_common

# Context keys chosen per workspace repo (derived from its git origin URL when
# possible, else from the workspace directory basename).
stevegt/decomk: DEFAULT Block20_go
```

Example `/var/decomk/conf/Makefile`:
```make
SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c

Block00_base:
	echo "base"
	touch $@

Block10_common: Block00_base
	echo "common"
	touch $@

Block20_go: Block10_common
	echo "go"
	touch $@
```

Run:
```bash
export DECOMK_HOME=/var/decomk
decomk plan
decomk run
```

After `decomk run`, stamp files exist under the printed `stampDir`, e.g.:
```text
/var/decomk/stamps/
  Block00_base        (stamp files created by make file targets)
  Block10_common
  Block20_go
```

The env export file is written to `<DECOMK_HOME>/env.sh`, e.g.:
```sh
# generated by decomk; do not edit
export DECOMK_HOME='/var/decomk'
export DECOMK_STAMPDIR='/var/decomk/stamps'
export DECOMK_DEV_USER='devuser'
export DECOMK_MAKE_USER='root'
export DECOMK_WORKSPACES='repo1 repo2'
export DECOMK_CONTEXTS='DEFAULT stevegt/decomk'
export DECOMK_PACKAGES='Block00_base Block10_common Block20_go'
...
```

`env.sh` is not debug-only: decomk builds one canonical env tuple sequence and
uses it for both `env.sh` generation and make invocation, so make and child
processes receive the same effective values.
This includes `PATH`: if a tuple sets `PATH=...`, that value applies to make
launcher resolution and to recipe child processes.

## Concepts

### Context

A **context key** selects a set of tokens from `decomk.conf`. Typical context
keys are:
- `DEFAULT` (common baseline)
- `owner/repo` (derived from the workspace repo’s `remote.origin.url` when available)
- `repo` (fallback)

In the typical devcontainer case, decomk applies multiple context keys in one
run:
- `DEFAULT` (when defined)
- plus one key per discovered workspace repo (when that key exists in config)

You can force a single context with `-context` / `DECOMK_CONTEXT`.

### Tokens

Each context key maps to a list of tokens. Tokens are one of:
- a macro reference (token matches another key in `decomk.conf`)
- a `NAME=value` tuple (passed to `make` on argv as a variable assignment)
- a make target name (everything else)

### Action args (isconf-style)

Positional args to `decomk plan/run` are interpreted like isconf:

- If an arg matches the name of a resolved tuple variable (for example `INSTALL`),
  decomk interprets that variable’s value as a whitespace-separated list of make
  targets.
- Otherwise, the arg is treated as a literal make target name.

This lets you define “what to run” as **action variables** rather than embedding
targets directly in the context expansion:

```conf
DEFAULT: INSTALL='install-neovim install-codex'
repo1: DEFAULT INSTALL='install-mob-consensus'
```

Usage:
```bash
decomk run INSTALL
decomk run install-neovim    # literal target fallback
```

If you provide any positional args, decomk uses them to select targets and
ignores any config-derived target tokens. If you provide no positional args,
decomk runs config-derived target tokens when present; otherwise it defaults to
`INSTALL` if defined.

### Stamps

`decomk` runs `make` in a **stamp directory** outside the workspace repo.

Make targets should usually be **file targets** (not `.PHONY`), whose recipes
end by creating/updating `$@` (often via `touch $@`). Because `make` is run in
the stamp directory, `$@` becomes a persistent “stamp file” that records that
the step has succeeded.

## How `decomk` works (algorithm)

`decomk plan` and `decomk run` share the same resolution pipeline:

1) Determine `DECOMK_HOME`
   - flag: `-home`
   - env: `DECOMK_HOME`
   - default: `/var/decomk`

2) Determine the starting directory (like `make -C`)
   - flag: `-C <dir>`
   - default: `.`
   - decomk changes directory to this path before resolving relative `-config` and `-makefile` paths

3) Determine the workspaces root directory to scan
   - flag: `-workspaces <dir>`
   - env: `DECOMK_WORKSPACES_DIR`
   - default: `/workspaces`

4) Stage-0 bootstrap (outside decomk core)
   - lifecycle tooling (for example `.devcontainer/postCreateCommand.sh`) ensures a `decomk` binary is available in `PATH`:
     - `DECOMK_TOOL_URI=go:<module>@<version>`: `go install <module>@<version>`
     - `DECOMK_TOOL_URI=git:<repo-url>[?ref=<git-ref>]`: clone/pull repo into `<DECOMK_HOME>/src/decomk`, optionally checkout ref, then `go install ./cmd/decomk`
   - lifecycle tooling syncs `DECOMK_CONF_URI=git:<repo-url>[?ref=<git-ref>]` into `<DECOMK_HOME>/conf`
   - `decomk plan/run` consumes this local state and does not clone/pull repos itself.

5) Load config definitions (`decomk.conf`)
   - **config repo** (optional): `<DECOMK_HOME>/conf/decomk.conf`
   - **explicit override** (optional): `-config <path>` or `DECOMK_CONFIG`

   Precedence is “last wins”:
   - config repo (lowest)
   - explicit `-config` / `DECOMK_CONFIG` (highest)

   Each of those sources is loaded as a *tree*:
   - the base `decomk.conf`
   - plus optional `decomk.d/*.conf` in lexical order
     - later files override earlier ones by key

7) Choose which context keys to apply
   - `-context <key>` / `DECOMK_CONTEXT` (must exist in config) forces a single context
   - otherwise:
     - scan `<workspacesDir>/*` for workspace repos
     - for each repo, try to find the most specific matching config key (first match wins):
       - `owner/repo` (derived from that workspace repo’s `remote.origin.url`)
       - `repo` (derived from origin URL or directory basename)
       - workspace directory basename
     - include a workspace’s key only if it exists in the loaded config
     - deduplicate keys across workspaces

8) Seed tokens
   - in the common case, seed tokens are:
     - `DEFAULT` (when defined)
     - plus the selected per-workspace keys (when any)

9) Expand macros (recursive)
   - if a token exactly matches a key in the config map, it is replaced by that
     key’s token list, recursively
   - unknown tokens remain literal
   - guardrails:
     - cycle detection
     - maximum depth (default 64; override with `-max-expand-depth`)

10) Partition expanded tokens
   - tuples: `NAME=value` where `NAME` matches `[A-Za-z_][A-Za-z0-9_]*`
   - targets: all other tokens 

11) Select make targets (isconf-style action args)
   - Build an “effective tuple map” from the tuple list (last assignment wins).
   - If positional args are provided:
     - for each arg:
       - if arg matches a tuple variable name: split its value on whitespace and append as targets
       - else: treat arg as a literal make target
     - Note: when positional args are provided, decomk ignores any config-derived target tokens.
   - If no positional args are provided:
     - if config-derived targets exist: use them (backward compatible)
     - else if `INSTALL` is defined and non-empty: split its value on whitespace and use that
     - else: pass no targets (make uses its default goal)
   - decomk exposes the selected targets as `DECOMK_PACKAGES` (exported in the env export file and passed to make).

12) Compute state paths
   - stamp dir (global):
     - `<DECOMK_HOME>/stamps/`
   - env export file (stable):
     - `<DECOMK_HOME>/env.sh`

13) Plan (`decomk plan`)
    - print the resolved plan (tuples + targets)
    - print the env exports that `run` would write (dry-run; does not write the env file)
    - run `make -n` in the stamp dir to show what would execute (dry-run)

14) Execute make (`decomk run`)
    - write the env export file:
      - `<DECOMK_HOME>/env.sh`
    - determine `Makefile` path:
      - `-makefile <path>` if set
      - otherwise, first existing of:
        - sibling of explicit `-config` (if set): `<dir-of-config>/Makefile`
        - `<DECOMK_HOME>/conf/Makefile`
    - acquire an exclusive global stamps lock:
      - `<DECOMK_HOME>/stamps/.lock`
    - ensure stamp dir exists, then **touch existing stamps** once (see below)
    - determine log root (first match wins):
      - `-log-dir <abs-path>` (overrides `DECOMK_LOG_DIR`)
      - `DECOMK_LOG_DIR`
      - `/var/log/decomk` (falls back to `<DECOMK_HOME>/log` when not writable)
    - create a per-run log dir (one per make invocation):
      - `<logRoot>/<runID>/`
      - `runID` includes sub-second time + pid for uniqueness
    - run:
      - `make -f <Makefile> <tuples...> <targets...>`
      - working directory = stamp dir
      - stdout/stderr are teed to `make.log` under the per-run log dir

## `decomk.conf` format

`decomk.conf` is intentionally small and deterministic:

- Whole-line comments start with `#`.
- Key lines are `key: token token token`.
  - The `:` must be followed by whitespace or end-of-line (this avoids treating
    `http://...` as a key line).
  - Keys cannot contain `=`.
- Any other non-empty, non-comment line is a continuation line and appends more
  tokens to the previous key.
- Tokens are whitespace-separated.
  - Single quotes may be used to include spaces inside a token:
    - `FOO='bar baz'` parses as one token `FOO=bar baz`
  - Backslash escapes the next rune when not in single quotes.
  - `NAME=$` is a passthrough sentinel for tuples:
    - if incoming env contains `NAME`, decomk uses that value
    - else if an earlier tuple already set `NAME`, decomk keeps that fallback
    - else decomk fails fast
- Incoming `DECOMK_*` environment variables are automatically carried into the
  canonical env export/make contract (unless later tuple/computed values
  override them).

## Makefile expectations and example

`decomk` runs `make` in the stamp directory and passes:
- tuples as argv variable assignments (`NAME=value`)
- targets as argv targets

This is the key idea: your “units of work” should be *make file targets* whose
recipes end by touching `$@`.

Example:
```make
SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c

# In decomk's execution model, targets below are files created in the stamp dir.

Block00_base:
	echo "base"
	touch $@

Block10_common: Block00_base
	echo "common"
	touch $@
```

Recommendation: touch `$@` *last* and only on success.

## Stamps and invalidation

### Why “touch existing stamps”?

Provisioning/bootstrapping often wants semantics closer to:
“run once unless explicitly invalidated”
than:
“re-run when a prerequisite timestamp changes”.

So before running `make`, `decomk` updates the mtime of existing (non-hidden)
regular files in the stamp dir, effectively making stamp deletion the main way
to force re-execution.

### How to force a step to re-run

Delete its stamp file in the stamp directory, then run again:
```bash
rm -f "$DECOMK_HOME/stamps/Block20_go"
decomk run ...
```

For “rerun everything”, delete the whole stamps directory (a future `decomk clean`
command will automate this).

## Persistent directory layout

By default, state lives under `/var/decomk`. You can override it with
`DECOMK_HOME` or `decomk -home`.

By default, per-run logs are written under `/var/log/decomk`. You can override
this with `DECOMK_LOG_DIR` or `decomk -log-dir`. If `/var/log/decomk` is not
writable and you did not explicitly override the log dir, decomk falls back to
`<DECOMK_HOME>/log`.

## CLI usage

```text
decomk init [flags]
decomk plan [flags] [ARGS...]
decomk run  [flags] [ARGS...]

ARGS:
  Action variable names (e.g. INSTALL) or literal make targets. If any ARGS are
  provided, decomk ignores config-derived target tokens and uses ARGS to select
  targets.

  Common flags for plan/run:
  -home <abs-path>          Override DECOMK_HOME
  -log-dir <abs-path>       Override DECOMK_LOG_DIR (default /var/log/decomk)
  -make-as-root=<bool>      Run make as root (default true; overrides DECOMK_MAKE_AS_ROOT)
  -C <dir>                  Starting directory (like make -C)
  -workspaces <dir>         Workspaces root directory to scan (default /workspaces; overrides DECOMK_WORKSPACES_DIR)
  -context <key>            Override context selection
  -config <path>            Explicit config file (overrides defaults)
  -makefile <path>          Explicit Makefile path
  -max-expand-depth <n>     Macro expansion depth limit (default 64)
  -v                        Verbose output

  Flags for init:
  -repo-root <path>         Repo root where .devcontainer files are written (default: current git repo root)
  -name <string>            devcontainer.json "name" value
  -conf-uri <uri>           DECOMK_CONF_URI value in devcontainer.json (git:...)
  -tool-uri <uri>           DECOMK_TOOL_URI value in devcontainer.json (go:... or git:...)
  -home <abs-path>          DECOMK_HOME value in devcontainer.json
  -log-dir <abs-path>       DECOMK_LOG_DIR value in devcontainer.json
  -run-args <string>        DECOMK_RUN_ARGS value in devcontainer.json
  -force                    Overwrite existing files when content differs
  -no-prompt                Do not prompt for unset values
```

## Makefile privilege model

By default, `decomk run` executes `make` as root via passwordless `sudo -n`
(unless `-make-as-root=false` / `DECOMK_MAKE_AS_ROOT=false`).

decomk intentionally uses a single privilege mode per invocation (it does not
split targets into “root phase” and “user phase”); this keeps stamp semantics
simple and repeatable.

When you need a user-scoped step (for example: dotfiles, `pipx` installs, or
other `$HOME` writes) while `make` is running as root, explicitly drop
privileges inside the Makefile using `runuser` (or `su`). decomk exports:

- `DECOMK_DEV_USER`: the dev user (the non-root user decomk expects to own state)
- `DECOMK_MAKE_USER`: the effective user `make` is running as (`root` or `DECOMK_DEV_USER`)

Example pattern:

```make
AS_DEV :=
ifneq ($(DECOMK_MAKE_USER),$(DECOMK_DEV_USER))
AS_DEV = runuser -u $(DECOMK_DEV_USER) --
endif

install-user-stuff:
	$(AS_DEV) ./scripts/install-user-stuff.sh
```

## Devcontainer notes

- `/var/decomk` (state) and `/var/log/decomk` (logs) should be writable by the dev user (or override with `DECOMK_HOME`/`DECOMK_LOG_DIR`).
  - In a Dockerfile, you typically want:
    - `RUN mkdir -p /var/decomk /var/log/decomk && chown -R $USER:$USER /var/decomk /var/log/decomk`
  - Alternatively, use a minimal lifecycle hook to run decomk directly; see `examples/devcontainer/postCreateCommand.sh`.
  - That hook performs stage-0 bootstrap by ensuring `decomk` is in `PATH`, syncing `DECOMK_CONF_URI`, then running `decomk`.
- The repo’s workspace path is host-dependent; prefer using
  `${containerWorkspaceFolder}` in `devcontainer.json` rather than assuming
  `/workspaces/<repo>`.
- Canonical scaffold sources are `cmd/decomk/templates/devcontainer.json.tmpl` and `cmd/decomk/templates/postCreateCommand.sh.tmpl`.
  - Generated files:
    - `examples/devcontainer/devcontainer.json`
    - `examples/devcontainer/postCreateCommand.sh`
    - `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json`
    - `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`
  - Regenerate with `go generate ./...` (or `make generate`).
- Companion static example file:
  - `examples/devcontainer/Dockerfile`

## Self-test harness

- Local DevPod Docker-provider validation lives under `examples/decomk-selftest/`.
  - Default tuple-override check:
    - `examples/decomk-selftest/devpod-local/run.sh`
  - Explicit tuple-action args:
    - `examples/decomk-selftest/devpod-local/run.sh TUPLE_VERIFY_TOOL TUPLE_VERIFY_CONF TUPLE_CONTEXT_OVERRIDE TUPLE_DEFAULT_SHARED`
  - The harness publishes temporary tool+config repos over `git://` and lets postCreate clone/pull them during stage-0 bootstrap.
  - Context selection is automatic from workspace repo name (`decomk`), with no explicit `-context` in harness calls.
  - Fixture config/make/scripts live under:
    - `examples/decomk-selftest/fixtures/confrepo/`
- Codespaces parity validation lives under `examples/decomk-selftest/codespaces/`.
  - Run:
    - `examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/<owner>/<conf-repo>.git`
  - The harness creates a fresh Codespace from the pushed branch under test, exports stage-0 URI vars, runs `examples/devcontainer/postCreateCommand.sh`, validates PASS/FAIL markers, runs stamp regression checks, then deletes the Codespace unless `--keep-on-fail` is set.
  - Codespaces parity harness requires local `HEAD` to match `origin/<branch>` (commit + push first).
  - Harness details and prerequisites are documented in:
    - `examples/decomk-selftest/README.md`
- Remote GCP-provider self-tests are intentionally deferred until a separate move-to-GCP decision is approved.

## Limitations (current MVP)

- No `status` / `clean` commands yet.
- Config parser is intentionally minimal (single quotes only; whole-line comments only).
- Stage-0 bootstrap expects `git` and `go` in the container (`postCreateCommand.sh` installs `decomk`, syncs repos, and runs `decomk`).
