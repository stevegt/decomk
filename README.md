# decomk

`decomk` is an isconf-inspired bootstrap wrapper for devcontainers.

## Philosophy

`decomk` separates concerns:

- **Policy** lives in shared config (`decomk.conf`): context expansion, tuple values, and default target composition.
- **Execution graph** lives in a shared `Makefile`: file targets in the stamp directory define idempotent, dependency-ordered work.
- **Stage-0 lifecycle files** (`.devcontainer/devcontainer.json`, `.devcontainer/postCreateCommand.sh`) are generated scaffolding and should be treated as managed bootstrap wrappers, not as the place to encode per-repo tool policy.

For deeper design background, see:

- `doc/decomk-design.md` (decomk behavior and selftest design)
- `doc/isconf-design.md` (isconf algorithm lineage)

## Current commands

- `decomk init` — scaffold stage-0 lifecycle files in `.devcontainer/`
- `decomk plan` — resolve tuples/targets + run `make -n` in the stamp directory
- `decomk run` — write env export file + run `make` in the stamp directory

## Step-by-step onboarding

### 1) Create one shared config repo for all managed containers

Create a git repo with at least:

- `decomk.conf`
- `Makefile`

Minimal example `decomk.conf`:

```conf
DEFAULT: Block00_base Block10_common
owner/repo-a: DEFAULT Block20_go
owner/repo-b: DEFAULT Block30_node
```

Minimal example `Makefile`:

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
	echo "go tools"
	touch $@
```

Push this repo somewhere reachable by your devcontainers and keep its clone URI ready as:

- `git:<repo-url>[?ref=<git-ref>]`

### 2) Run `decomk init` in each repo you want managed

Install decomk on your own machine:

```bash
go install github.com/stevegt/decomk/cmd/decomk@stable
```

Then in each workspace repo:

```bash
decomk init -conf-uri git:<your-shared-conf-repo-url>
```

This writes:

- `.devcontainer/devcontainer.json`
- `.devcontainer/postCreateCommand.sh`

### 3) Start/rebuild the devcontainer

The generated post-create hook:

1. ensures `decomk` is available in `PATH` from `DECOMK_TOOL_URI`,
2. syncs config repo from `DECOMK_CONF_URI` into `<DECOMK_HOME>/conf`,
3. runs `decomk run ${DECOMK_RUN_ARGS:-all}`.

## `decomk init` safety and overwrite policy

`decomk init` is conservative by default:

- If **either** target stage-0 file already exists, `decomk init` fails.
- It does **not** overwrite existing files in default mode.
- It does **not** write alternate temp merge files in default mode.

This is safe to run in existing repos because it will stop before changing managed files.

If you intentionally want to regenerate/replace files, use force:

- `-f` (alias)
- `-force`

Recommended reconciliation workflow when files already exist:

```bash
git commit -m "Checkpoint existing devcontainer files"
decomk init -f -conf-uri git:<your-shared-conf-repo-url>
git difftool -- .devcontainer/devcontainer.json .devcontainer/postCreateCommand.sh
```

## Stage-0 template ownership

Canonical sources:

- `cmd/decomk/templates/devcontainer.json.tmpl`
- `cmd/decomk/templates/postCreateCommand.sh.tmpl`

Generated copies:

- `examples/devcontainer/devcontainer.json`
- `examples/devcontainer/postCreateCommand.sh`
- `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json`
- `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`

Regenerate/check:

```bash
make generate
make check-generated
```

## Key stage-0 environment contract

Primary stage-0 vars in `devcontainer.json`:

- `DECOMK_TOOL_URI` — tool source (`go:` or `git:` URI)
- `DECOMK_CONF_URI` — config source (`git:` URI)
- `DECOMK_HOME` — state root (default `/var/decomk`)
- `DECOMK_LOG_DIR` — run-log root (default `/var/log/decomk`)
- `DECOMK_RUN_ARGS` — action args passed to `decomk run`

Legacy variable-name migration mapping is documented in:

- `TODO/001-decomk-devcontainer-tool-bootstrap.md` (`Legacy stage-0 variable migration mapping`)

## Run/plan quick examples

```bash
decomk plan
decomk run
```

With explicit local files for experimentation:

```bash
DECOMK_HOME=/tmp/decomk \
  decomk plan -config ./decomk.conf -makefile ./Makefile -context myrepo

DECOMK_HOME=/tmp/decomk DECOMK_LOG_DIR=/tmp/decomk/log \
  decomk run -config ./decomk.conf -makefile ./Makefile -context myrepo
```

`decomk run` writes `<DECOMK_HOME>/env.sh` and runs make in `<DECOMK_HOME>/stamps`.

## Logging and state defaults

- state root: `/var/decomk` (override `DECOMK_HOME` / `-home`)
- run logs: `/var/log/decomk` (override `DECOMK_LOG_DIR` / `-log-dir`)
- default log-root fallback: `<DECOMK_HOME>/log` when default `/var/log/decomk` is not writable

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
  -force                    Overwrite existing stage-0 files even when they already exist
  -f                        Alias for -force
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

- Root convenience targets:
  - `make selftest-devpod`
  - `make selftest-codespaces`
  - `make selftest-codespaces-clean`
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
    - `examples/decomk-selftest/codespaces/run.sh`
  - The harness creates a fresh Codespace from the pushed branch under test, auto-builds a fixture config repo inside the Codespace, exports stage-0 URI vars, runs `examples/devcontainer/postCreateCommand.sh`, validates PASS/FAIL markers, runs stamp regression checks, then deletes the Codespace unless `--keep-on-fail` is set.
  - Local harness artifacts under `/tmp/decomk-codespaces.*` are preserved by default for inspection; pass `--cleanup` to remove them on success.
  - Diagnostics artifacts are explicit and completion-marked:
    - `diagnostics-summary.txt` (step-by-step status)
    - `diag-<step>.rc`, `diag-<step>.stdout.log`, `diag-<step>.stderr.log`
    - `diagnostics.complete` (written when artifact collection is finished)
  - The selftest devcontainer enables `ghcr.io/devcontainers/features/sshd:1` because harness execution depends on `gh codespace ssh`.
  - Machine selection auto-resolves from the repository-allowed Codespaces machine list (prefers `basicLinux32gb` when available); override with:
    - `examples/decomk-selftest/codespaces/run.sh --machine <machine-name>`
  - Optional override for external fixture config source:
    - `examples/decomk-selftest/codespaces/run.sh --conf-uri git:https://github.com/<owner>/<conf-repo>.git`
  - Codespaces parity harness requires local `HEAD` to match `origin/<branch>` (commit + push first).
  - Harness details and prerequisites are documented in:
    - `examples/decomk-selftest/README.md`
- Remote GCP-provider self-tests are intentionally deferred until a separate move-to-GCP decision is approved.

## Limitations (current MVP)

- No `status` / `clean` commands yet.
- Config parser is intentionally minimal (single quotes only; whole-line comments only).
- Stage-0 bootstrap expects `git` and `go` in the container (`postCreateCommand.sh` installs `decomk`, syncs repos, and runs `decomk`).
