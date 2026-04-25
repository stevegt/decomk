# decomk

`decomk` is an [isconf](https://en.wikipedia.org/wiki/ISconf)-inspired bootstrap wrapper for devcontainers.

## Philosophy

`decomk` separates concerns:

- **Policy** lives in shared config (`decomk.conf`): context expansion, tuple values, and action-variable composition.
- **Execution graph** lives in a `Makefile` that is shared across multiple devcontainer repos: file targets in the stamp directory define idempotent, dependency-ordered work.
- **Stage-0 lifecycle files** (`.devcontainer/devcontainer.json`, `.devcontainer/decomk-stage0.sh`) are generated scaffolding and should be treated as managed bootstrap wrappers, not as the place to encode per-repo tool policy.

For deeper design background, see:

- `doc/decomk-design.md` (decomk behavior and selftest design)
- `doc/isconf-design.md` (isconf algorithm lineage)

## Current commands

- `decomk init -conf` — scaffold a shared conf repo (`decomk.conf` + `Makefile` + producer `.devcontainer/`)
- `decomk init` — scaffold stage-0 lifecycle files in `.devcontainer/` using shared producer defaults for tool/home/log values while taking remote identity from the image
- `decomk version` — print the decomk CLI version string
- `decomk plan` — resolve tuples/targets + run `make -n` in the stamp directory
- `decomk run` — write env export file + run `make` in the stamp directory
- `decomk checkpoint` — build/push/tag shared checkpoint images for the `updateContent` phase

## Versioning and release

- `VERSION` at repo root is the canonical CLI release version source.
- `go generate ./...` updates `cmd/decomk/version_generated.go` from `VERSION`.
- `decomk version` prints the generated value at runtime (unless overridden by build-time ldflags).

Minor release workflow:

```bash
make release-minor
```

This runs `scripts/release.sh minor`, which:

1. refuses to run if the repo is dirty,
2. reads and bumps `VERSION` minor (`vMAJOR.MINOR[.PATCH] -> vMAJOR.(MINOR+1).0`),
3. regenerates `cmd/decomk/version_generated.go`,
4. commits the version bump,
5. creates a matching git tag,
6. pushes branch and tags.

## Step-by-step onboarding

### 1) Bootstrap one shared conf repo with `decomk init -conf`

Install decomk on your own machine:

```bash
go install github.com/stevegt/decomk/cmd/decomk@latest
```

In your shared conf repo:

```bash
decomk init -conf -conf-uri git:<your-shared-conf-repo-url> -image mcr.microsoft.com/devcontainers/base:ubuntu-24.04
```

This writes a starter tree:

- `decomk.conf`
- `Makefile`
- `README.md`
- `bin/hello-world.sh`
- `.devcontainer/devcontainer.json`
- `.devcontainer/decomk-stage0.sh`
- `.devcontainer/Dockerfile`

### 2) Customize and push the shared conf repo

- Edit `decomk.conf` contexts/tuples for your org/repo policy.
- Edit `Makefile` targets for idempotent shared setup work.
- Push the repo and keep its URI ready as:
  - `git:<repo-url>[?ref=<git-ref>]`

### 3) Run `decomk init` in each consumer repo

In each workspace repo you want managed:

```bash
decomk init -conf-uri git:<your-shared-conf-repo-url>
```

This writes:

- `.devcontainer/devcontainer.json`
- `.devcontainer/decomk-stage0.sh`

### 4) Start/rebuild the devcontainer

The generated stage-0 hooks:

1. ensures `decomk` is available in `PATH` from `DECOMK_TOOL_URI`,
2. syncs config repo from `DECOMK_CONF_URI` into `<DECOMK_HOME>/conf`,
3. runs `decomk run <action-args>` (defaulting to lifecycle phase selector `updateContent`/`postCreate`).

## `decomk init -conf` safety and overwrite policy

`decomk init -conf` is conservative by default:

- If **any** managed conf-repo file already exists, `decomk init -conf` fails.
- It does **not** overwrite existing files in default mode.
- It does **not** write alternate temp merge files in default mode.

If you intentionally want to regenerate/replace files, use force:

- `-f` (alias)
- `-force`

When `-f` is used and `.devcontainer/Dockerfile` already exists, `decomk init -conf`
reuses these values as prompt defaults:

- first `FROM` image
- `ENV DECOMK_REMOTE_USER=...`
- `ENV DECOMK_REMOTE_UID=...`

If Dockerfile parsing fails, init prints a warning and continues with fallback defaults.

Recommended reconciliation workflow when files already exist:

```bash
git commit -m "Checkpoint existing conf-repo files"
decomk init -conf -f -conf-uri git:<your-shared-conf-repo-url>
git difftool -- decomk.conf Makefile README.md .devcontainer/devcontainer.json .devcontainer/decomk-stage0.sh .devcontainer/Dockerfile
```

## `decomk init` safety and overwrite policy

`decomk init` is conservative by default:

- If **either** target stage-0 file already exists, `decomk init` fails.
- It does **not** overwrite existing files in default mode.
- It does **not** write alternate temp merge files in default mode.

This is safe to run in existing repos because it will stop before changing managed files.

If you intentionally want to regenerate/replace files, use force:

- `-f` (alias)
- `-force`

When `-f` is used and `.devcontainer/devcontainer.json` already exists,
`decomk init` reuses existing stage-0 values as defaults so reruns do not
require re-entering the same configuration. For consumer init, producer
defaults then override local defaults for `DECOMK_TOOL_URI`, `DECOMK_HOME`, and
`DECOMK_LOG_DIR` (CLI still wins).

Consumer `decomk init` requires a reachable `DECOMK_CONF_URI` (`git:...`) and
clones the producer conf repo to inherit shared defaults for `DECOMK_TOOL_URI`,
`DECOMK_HOME`, and `DECOMK_LOG_DIR` (precedence: CLI > producer > local existing > built-in).
If clone/read fails, init fails fast rather than silently falling back.

Generated stage-0 scripts verify that the runtime process identity matches
`DECOMK_REMOTE_USER` / `DECOMK_REMOTE_UID` before escalating to root; mismatches
fail fast to avoid ambiguous ownership drift. Those identity vars should come
from the image (for example Dockerfile `ENV`), not from generated consumer
`devcontainer.json` metadata.

Recommended reconciliation workflow when files already exist:

```bash
git commit -m "Checkpoint existing devcontainer files"
decomk init -f -conf-uri git:<your-shared-conf-repo-url>
git difftool -- .devcontainer/devcontainer.json .devcontainer/decomk-stage0.sh
```

## Stage-0 template ownership

Canonical sources:

- `cmd/decomk/templates/devcontainer.json.tmpl`
- `cmd/decomk/templates/decomk-stage0.sh.tmpl`

Generated copies:

- `examples/devcontainer/devcontainer.json`
- `examples/devcontainer/decomk-stage0.sh`
- `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json`
- `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/decomk-stage0.sh`

Regenerate/check:

```bash
make generate
make check-generated
```

`init -conf` canonical sources:

- `cmd/decomk/templates/confrepo.decomk.conf.tmpl`
- `cmd/decomk/templates/confrepo.Makefile.tmpl`
- `cmd/decomk/templates/confrepo.README.md.tmpl`
- `cmd/decomk/templates/confrepo.hello-world.sh.tmpl`
- `cmd/decomk/templates/confrepo.devcontainer.json.tmpl`
- `cmd/decomk/templates/confrepo.Dockerfile.tmpl`

Generated copy:

- `examples/confrepo/*`

## Key stage-0 environment contract

Primary stage-0 vars in `devcontainer.json`:

- `DECOMK_TOOL_URI` — tool source (`go:` or `git:` URI)
- `DECOMK_CONF_URI` — config source (`git:` URI)
- `DECOMK_HOME` — state root (default `/var/decomk`)
- `DECOMK_LOG_DIR` — run-log root (default `/var/log/decomk`)
- `DECOMK_FAIL_NOBOOT` — stage-0 failure policy (`false` default: continue boot after writing diagnostics; `true`: fail startup)

Generated lifecycle hooks call one script with explicit phase args:

- `updateContentCommand`: `bash .devcontainer/decomk-stage0.sh updateContent`
- `postCreateCommand`: `bash .devcontainer/decomk-stage0.sh postCreate`

When stage-0 fails, generated script behavior is:

- `DECOMK_FAIL_NOBOOT=true`: exit non-zero and fail startup.
- unset/false: write phase-specific failure marker/log under `<DECOMK_HOME>/stage0/failure/`, write a MOTD hint (or fallback hint file), then return success so container boot continues.

Legacy variable-name migration mapping is documented in:

- `TODO/001-decomk-devcontainer-tool-bootstrap.md` (`Legacy stage-0 variable migration mapping`)

## Run/plan quick examples

```bash
decomk plan INSTALL
decomk run INSTALL
```

With explicit local files for experimentation:

```bash
DECOMK_HOME=/tmp/decomk \
  decomk plan INSTALL -config ./decomk.conf -makefile ./Makefile -context myrepo

DECOMK_HOME=/tmp/decomk DECOMK_LOG_DIR=/tmp/decomk/log \
  decomk run INSTALL -config ./decomk.conf -makefile ./Makefile -context myrepo
```

`decomk run` writes `<DECOMK_HOME>/env.sh` and runs make in `<DECOMK_HOME>/stamps`.

## Checkpoint quick examples

```bash
# Build a local checkpoint candidate by running devcontainer prebuild lifecycle.
decomk checkpoint build -workspace-folder . -config .devcontainer/devcontainer.json -tag ghcr.io/acme/base:block10-candidate

# Publish candidate to immutable + testing tags.
decomk checkpoint push ghcr.io/acme/base:block10-candidate \
  ghcr.io/acme/base:block10-20260420 \
  ghcr.io/acme/base:testing

# After external/manual tests pass, move stable explicitly.
decomk checkpoint tag -m ghcr.io/acme/base:block10-20260420 ghcr.io/acme/base:stable
```

`push`/`tag` fail if a destination tag already exists unless `-m` is set.
`checkpoint build` is verbose by default (lifecycle logs on stderr); pass `-q`
to suppress lifecycle log output.

For operator/CI checkpoint rollout handoff details:

- `TODO/011-single-path-checkpoints.md` (`011.7 Operator/CI handoff contract`) — canonical step-by-step contract and artifact requirements.
- `doc/image-management.md` — design rationale and lifecycle context.

## Consumer selector policy (TODO 010)

Consumer repos should use one canonical `.devcontainer/devcontainer.json`
and select checkpoint images via `image:` tags:

- **Channel-following** (for example `:stable` or `:testing`) when teams
  want automatic uptake of tested promotions.
- **Immutable pinning** (for example `:block10-20260420`) when teams need
  a controlled freeze window.

No `.devcontainer/BlockXX/...` profile-path switching is required in the
current model. Maintainers update selector policy via normal repo commits.
See `TODO/010-codespaces-block-prebuild-profiles.md` for policy details.

## Logging and state defaults

- state root: `/var/decomk` (override `DECOMK_HOME` / `-home`)
- run logs: `/var/log/decomk` (override `DECOMK_LOG_DIR` / `-log-dir`)
- default log-root fallback: `<DECOMK_HOME>/log` when default `/var/log/decomk` is not writable

## MOTD run summaries (`DECOMK_MOTD_PHASES`)

`decomk run` can publish post-run MOTD files when the tuple
`DECOMK_MOTD_PHASES` is present in resolved config.

Format:

- CSV entries in `NN:phase` form, for example:
  - `DECOMK_MOTD_PHASES='88:version,93:updateContent,94:postCreate'`
- `NN` must be exactly two digits.
- `phase` may contain letters, numbers, `_`, `-`, and `.`.

Behavior:

- If `DECOMK_MOTD_PHASES` is unset, no run-summary MOTD files are written.
- If the current `DECOMK_STAGE0_PHASE` has a mapping, decomk writes:
  - `/etc/motd.d/<NN>-decomk-<phase>`
- If `version` is mapped (for example `88:version`), decomk also writes:
  - `/etc/motd.d/<NN>-decomk-version`
  - content starts with a blank line, then `decomk version: <value>`.
- If `/etc/motd.d` cannot be written, decomk falls back to:
  - `<DECOMK_HOME>/stage0/failure/<same-filename>`
- Invalid mapping syntax is surfaced as a run warning.

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

Bare RHS tokens that are not tuples must resolve to keys defined in `decomk.conf`.
Unknown bare RHS tokens are rejected as config errors.

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

`decomk plan` and `decomk run` require at least one positional action arg.

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
   - lifecycle tooling (for example `.devcontainer/decomk-stage0.sh`) ensures a `decomk` binary is available in `PATH`:
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
    - unknown bare RHS tokens are rejected before expansion (`decomk.conf` must be tuple-or-key only)
    - guardrails:
      - cycle detection
      - maximum depth (default 64; override with `-max-expand-depth`)

10) Partition expanded tokens
    - tuples: `NAME=value` where `NAME` matches `[A-Za-z_][A-Za-z0-9_]*`
    - targets: must be empty (non-empty output is treated as invalid config)

11) Select make targets (isconf-style action args)
    - Build an “effective tuple map” from the tuple list (last assignment wins).
    - Positional args are required.
    - For each arg:
      - if arg matches a tuple variable name: split its value on whitespace and append as targets
      - else: treat arg as a literal make target
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
    - optionally write MOTD summaries when `DECOMK_MOTD_PHASES` is configured:
      - `<NN>-decomk-<DECOMK_STAGE0_PHASE>` when current phase is mapped
      - `<NN>-decomk-version` when `version` is mapped
      - fallback under `<DECOMK_HOME>/stage0/failure/` when `/etc/motd.d` is not writable

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
- `DECOMK_MOTD_PHASES` is a regular tuple value that controls optional run MOTD
  writes (`NN:phase` CSV); example:
  - `DEFAULT: DECOMK_MOTD_PHASES='88:version,93:updateContent,94:postCreate'`

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
decomk version
decomk plan [flags] [ARGS...]
decomk run  [flags] [ARGS...]

ARGS:
  Action variable names (e.g. INSTALL) or literal make targets.
  ARGS are required for both `decomk plan` and `decomk run`.

  Common flags for plan/run:
  -home <abs-path>          Override DECOMK_HOME
  -log-dir <abs-path>       Override DECOMK_LOG_DIR (default /var/log/decomk)
  -C <dir>                  Starting directory (like make -C)
  -workspaces <dir>         Workspaces root directory to scan (default /workspaces; overrides DECOMK_WORKSPACES_DIR)
  -context <key>            Override context selection
  -config <path>            Explicit config file (overrides defaults)
  -makefile <path>          Explicit Makefile path
  -max-expand-depth <n>     Macro expansion depth limit (default 64)
  -v                        Verbose output

  Flags for init:
  -repo-root <path>         Repo root where .devcontainer files are written (default: current git repo root)
  -conf                     Producer mode: scaffold shared conf repo starter files at repo root
  -name <string>            devcontainer.json "name" value (default: repo basename)
  -image <ref>              consumer mode: devcontainer image value when no build dockerfile is configured; producer mode (-conf): Dockerfile FROM base image
  -conf-uri <uri>           DECOMK_CONF_URI value in devcontainer.json (git:...; required in consumer mode)
  -tool-uri <uri>           DECOMK_TOOL_URI value in devcontainer.json (go:... or git:...)
  -home <abs-path>          DECOMK_HOME value in devcontainer.json
  -log-dir <abs-path>       DECOMK_LOG_DIR value in devcontainer.json
  -remote-user <name>       DECOMK_REMOTE_USER value for producer Dockerfile ENV (producer mode)
  -remote-uid <uid>         DECOMK_REMOTE_UID value for producer Dockerfile ENV (producer mode)
  -fail-no-boot <value>     DECOMK_FAIL_NOBOOT value in devcontainer.json (true/false/1/0/yes/no/on/off)
  -force                    Overwrite existing stage-0 files even when they already exist
  -f                        Alias for -force
  -no-prompt                Do not prompt for unset values
```

## Makefile privilege model

`decomk run` now requires root and does not do its own sudo fallback logic.
The generated stage-0 hook (`.devcontainer/decomk-stage0.sh`) handles one
non-interactive re-exec via `sudo -n -E` before calling `decomk run`.

decomk intentionally uses a single privilege mode per invocation (it does not
split targets into “root phase” and “user phase”); this keeps stamp semantics
simple and repeatable.

When you need a user-scoped step (for example: dotfiles, `pipx` installs, or
other `$HOME` writes) while `make` is running as root, explicitly drop
privileges inside the Makefile using `runuser` (or `su`). decomk exports:

- `DECOMK_REMOTE_USER`: the dev user (the non-root user decomk expects to own state)
- `DECOMK_MAKE_USER`: the effective user `make` is running as (`root`)

Example pattern:

```make
AS_DEV :=
ifneq ($(DECOMK_MAKE_USER),$(DECOMK_REMOTE_USER))
AS_DEV = runuser -u $(DECOMK_REMOTE_USER) --
endif

install-user-stuff:
	$(AS_DEV) ./scripts/install-user-stuff.sh
```

## Devcontainer notes

- `/var/decomk` (state) and `/var/log/decomk` (logs) should be writable by the dev user (or override with `DECOMK_HOME`/`DECOMK_LOG_DIR`).
  - In a Dockerfile, you typically want:
    - `RUN mkdir -p /var/decomk /var/log/decomk && chown -R $USER:$USER /var/decomk /var/log/decomk`
  - Export `DECOMK_REMOTE_USER` and `DECOMK_REMOTE_UID` in the image (for example with Dockerfile `ENV`) so stage-0 identity checks are explicit and deterministic.
  - Alternatively, use a minimal lifecycle hook to run decomk directly; see `examples/devcontainer/decomk-stage0.sh`.
  - That hook performs stage-0 bootstrap by ensuring `decomk` is in `PATH`, syncing `DECOMK_CONF_URI`, then running `decomk`.
- The repo’s workspace path is host-dependent; prefer using
  `${containerWorkspaceFolder}` in `devcontainer.json` rather than assuming
  `/workspaces/<repo>`.
- Canonical scaffold sources are `cmd/decomk/templates/devcontainer.json.tmpl` and `cmd/decomk/templates/decomk-stage0.sh.tmpl`.
  - Generated files:
    - `examples/devcontainer/devcontainer.json`
    - `examples/devcontainer/decomk-stage0.sh`
    - `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json`
    - `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/decomk-stage0.sh`
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
  - The harness creates a fresh Codespace from the pushed branch under test, auto-builds a fixture config repo inside the Codespace, exports stage-0 URI vars, runs `examples/devcontainer/decomk-stage0.sh postCreate`, validates PASS/FAIL markers, runs stamp regression checks, then deletes the Codespace unless `--keep-on-fail` is set.
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
- Experimental lifecycle-evidence spike (POC, not a canonical decomk example):
  - `examples/phase-eval/README.md`

## Limitations (current MVP)

- No `status` / `clean` commands yet.
- Config parser is intentionally minimal (single quotes only; whole-line comments only).
- Stage-0 bootstrap expects `git` and `go` in the container (`decomk-stage0.sh` installs `decomk`, syncs repos, and runs `decomk`).
