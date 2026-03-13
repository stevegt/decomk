# TODO 001 - decomk: isconf-like devcontainer tool bootstrap

## Decision Intent Log

ID: DI-001-20260309-172358
Date: 2026-03-09 17:23:58
Status: active
Decision: Default to running `make` as root (via passwordless `sudo -n` when needed), export `DECOMK_DEV_USER`/`DECOMK_MAKE_USER` so Makefiles can drop privileges explicitly, and avoid guessing dev usernames in the reference devcontainer bootstrap (warn + run as root when unknown).
Intent: Keep devcontainer bootstrap non-interactive (no password prompts), avoid sprinkling `sudo` throughout Makefiles, keep stamp semantics predictable by using a single privilege mode per run, and avoid mis-identifying the dev user by using only reliable inference signals.
Constraints: No sudo password prompts; `decomk plan` should work without sudo; stamps must remain writable by the dev user across runs; when running under a root lifecycle hook, user inference must not guess incorrect usernames.
Affects: `cmd/decomk/main.go`, `makeexec/makeexec.go`, `README.md`, `examples/devcontainer/postCreateCommand.sh`.

ID: DI-001-20260311-161825
Date: 2026-03-11 16:18:25
Status: active
Decision: Add a first-class `decomk init` subcommand that installs `.devcontainer/devcontainer.json` and `.devcontainer/postCreateCommand.sh` from templates embedded in the decomk binary, using CLI flags or interactive prompts to populate workspace-specific values.
Intent: Make stage-0 bootstrap setup repeatable and low-friction in new repos while keeping the bootstrap script generic and production-identical.
Constraints: Generated files must land under `.devcontainer/`, template defaults must preserve current stage-0 bootstrap behavior, and users must be able to run non-interactively in automation.
Affects: `cmd/decomk/main.go`, `cmd/decomk/main_test.go`, `cmd/decomk/templates/*`, `README.md`, `examples/devcontainer/*`.

ID: DI-001-20260311-163942
Date: 2026-03-11 16:39:42
Status: active
Decision: Make stage-0 tool bootstrap install-first by default (`go install`), with optional clone mode that runs `go install ./cmd/decomk` in a pulled repo.
Intent: Remove unnecessary tool-repo checkout state from the default path while still supporting selftest and local-branch workflows that need git clone/pull semantics.
Constraints: Do not force a custom install path; let `go install` use standard `GOBIN`/`GOPATH/bin` behavior, keep the bootstrap script production-generic, and preserve config-repo clone/pull behavior.
Affects: `examples/devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`, `cmd/decomk/templates/postCreateCommand.sh.tmpl`, `cmd/decomk/templates/devcontainer.json.tmpl`, `cmd/decomk/init.go`, `README.md`.

ID: DI-001-20260311-164841
Date: 2026-03-11 16:48:41
Status: active
Decision: When `decomk init` is run without `-repo-root`, resolve the target repo to the git toplevel of the current working directory instead of using `.`.
Intent: Make default scaffold placement match user expectation in nested paths inside a repo and avoid accidentally scaffolding the wrong directory.
Constraints: Preserve explicit `-repo-root` override behavior, and return a clear error when the current directory is not inside a git work tree.
Affects: `cmd/decomk/init.go`, `cmd/decomk/init_test.go`, `README.md`.

ID: DI-001-20260311-175002
Date: 2026-03-11 17:50:02
Status: active
Decision: Make scaffold file writes atomic in `decomk init`, and validate `DECOMK_TOOL_INSTALL_PKG` only when `DECOMK_TOOL_MODE=install`.
Intent: Prevent partial scaffold files on interruption and avoid unnecessary validation coupling between clone mode and install package settings.
Constraints: Keep overwrite semantics unchanged (`-force`), preserve file modes, and keep error messages clear for invalid mode-specific inputs.
Affects: `cmd/decomk/init.go`, `cmd/decomk/init_test.go`.

ID: DI-001-20260312-130300
Date: 2026-03-12 13:03:00
Status: active
Decision: Use one canonical scaffold template contract (`cmd/decomk/templates/*`) for `decomk init` and generated example files, with `go generate` + `scaffoldgen` + tests enforcing drift checks.
Intent: Keep production and selftest bootstrap files synchronized by construction and remove hand-maintained duplication across examples.
Constraints: Preserve embedded-template behavior for `decomk init`, keep generated outputs deterministic, and provide both developer and CI entrypoints for regeneration/checking.
Affects: `scaffold/scaffold.go`, `cmd/decomk/init.go`, `cmd/scaffoldgen/main.go`, `cmd/decomk/generate_scaffolds.go`, `cmd/decomk/scaffold_sync_test.go`, `examples/devcontainer/*`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/*`, `Makefile`, `README.md`, `doc/decomk-design.md`, `examples/decomk-selftest/README.md`.

ID: DI-001-20260312-141200
Date: 2026-03-12 14:12:00
Status: active
Decision: Rename internal generator/renderer naming from `scaffold`/`scaffoldgen` to `stage0`/`stage0gen`.
Intent: Make names match decomk architecture language (“stage-0 bootstrap”) and reduce ambiguity with runtime `decomk.conf` context config.
Constraints: No behavior changes; preserve template contract, generation flow (`go generate`), and drift checks.
Affects: `stage0/stage0.go`, `cmd/stage0gen/main.go`, `cmd/decomk/generate_stage0.go`, `cmd/decomk/stage0_sync_test.go`, `cmd/decomk/init.go`, `cmd/decomk/init_test.go`, `Makefile`, `doc/decomk-design.md`.
Supersedes: DI-001-20260312-130300

ID: DI-001-20260313-183500
Date: 2026-03-13 18:35:00
Status: active
Decision: Make `examples/devcontainer/devcontainer.json` a standalone runnable example by generating a `build` entry (`Dockerfile`, context `"."`) and `remoteUser: "dev"`, and add a companion `examples/devcontainer/Dockerfile`.
Intent: Prevent devcontainer startup failures for users who copy the example directly by providing an explicit image build path and a realistic non-root runtime user out of the box.
Constraints: Keep stage-0 template generation as the source of truth, preserve existing container env defaults, and avoid changing selftest devcontainer behavior.
Affects: `stage0/stage0.go`, `examples/devcontainer/devcontainer.json`, `examples/devcontainer/Dockerfile`, `README.md`.

## Goal

Create an isconf-inspired “context -> target groups + vars”
bootstrap so a devcontainer-based workspace (Codespaces now; other
hosts later) can automatically install **shared** dev tools plus
**repo-specific** tools using a Makefile, without needing
hostname-based host configuration or Perl.

## Desired properties

- Deterministic: the same repo creates the same toolset by default.
- Layered: `DEFAULT` (shared tools) + `<repo>` (repo-specific tools).
- Idempotent: re-running is safe (good for `postCreateCommand` +
  `postStartCommand`).
    - Working definition for this tool: repeated runs should converge
      on the same declared toolset without breaking a working
      environment. Side effects are expected (and are the point) and
      OK if they are safe on re-run (stamps prevent re-runs of those
      that aren't safe).
    - Also see the wikipedia idempotent page for more nuanced
      properties (e.g., “side effects” like package installations must
      be idempotent). Also see the monoid wikipedia page.
- Auditable: resolved variables/targets are written to a file for
  review/debugging.
- No hostname fallback: identity comes from Codespaces env (or
  explicit overrides).

## Codespaces vs the Dev Container standard (containers.dev)

Terminology we should keep straight in this TODO:

- **Dev Container standard** (https://containers.dev/) is an *open
  specification* for describing a development container via
  `.devcontainer/devcontainer.json` (and optional `Dockerfile`),
  including build/run settings and lifecycle hooks (e.g.,
  `postCreateCommand`).
  - It is primarily about “how an editor attaches to a container and
    what should run inside it”.
  - It does **not** define how compute is provisioned, how ports are
    exposed to the internet, org policy/quotas, billing, or secrets
    management. Those are platform concerns.
  - It also does not mandate a specific in-container workspace path
    like `/workspaces`. Use the devcontainer concepts (`workspaceFolder`
    / `workspaceMount` and `${containerWorkspaceFolder}`) rather than
    hardcoding Codespaces-specific paths.

- **GitHub Codespaces** is a *hosted product/platform* that provisions a
  VM + devcontainer, uses `devcontainer.json` as the repo contract, and
  adds GitHub-specific behaviors:
  - machine sizing / provisioning (CPU/RAM/disk), retention, and
    prebuild/caching
  - identity + auth to GitHub, plus environment variables that describe
    the repo/workspace
  - port forwarding via a GitHub-managed proxy/URLs + per-port
    visibility controls (private/org/public), subject to org policy
  - lifecycle management via `gh codespace ...` and a GitHub API
  - a conventional workspace location: the repo is typically mounted
    under `/workspaces/<repo>` (don’t assume this outside Codespaces)

Multi-repo workspaces:
- The Dev Container spec doesn’t define “clone multiple repos into one
  workspace” as a first-class feature. The portable approach is to
  implement this in lifecycle hooks (e.g., `postCreateCommand`) as an
  idempotent bootstrap step.
- Codespaces can make this easier in practice (e.g., by granting the
  codespace/prebuild access to additional repos), but the behavior is
  still best expressed as “bootstrap clones N repos” so it works in
  non-Codespaces hosts (DevPod, self-host, Promisegrid).

Implication for `decomk` (this TODO’s tool):
- We should treat **Dev Container** as the portable hook point
  (`postCreateCommand` runs `decomk make ...`), and treat Codespaces
  as *one* platform that provides useful default identity (`owner/repo`,
  codespace name, etc.).
- For non-Codespaces runs (local devcontainers, DevPod, GCP self-host),
  require an explicit override env var (`DECOMK_CONTEXT`, etc.) so we
  don’t bake GitHub-specific assumptions into the core algorithm.

## Context identity (devcontainers)

Inputs (preferred order):
1. `GITHUB_REPOSITORY` (required in Codespaces; `owner/repo`)  --
   everything after the slash is the repo name, which is a natural
   context key. BUT if there is a makefile target for the full
   owner/repo, that should take precedence over the repo name alone
   (e.g., `stevegt/grokker` should take precedence over `grokker` if
   both exist). 
    - Decision: handle precedence in the wrapper by trying a list
      of candidate context keys in order (override var → `owner/repo` →
      `repo` → `DEFAULT`); the config file can simply define whichever
      keys are needed.
2. Optional override: `DECOMK_CONTEXT` / `DECOMK_REPO` (for local dev and
tests)
   - Update: the tool is now named `decomk` and is not Codespaces-only.
     Prefer `DECOMK_CONTEXT` / `DECOMK_REPO` and the `DECOMK_*` prefix.
   - Historical naming brainstorm (from when this was Codespaces-centric):
     - spacemaker: it "makes" the "space" (Codespace) ready to work
     - spacemakr: more unique
     - spaceconf: like isconf but for Codespaces
     - spaceconfig: more unique but less catchy

Derived:
- `REPO_NAME=${GITHUB_REPOSITORY##*/}`
- `CODESPACE_NAME=$CODESPACE_NAME` (optional; useful for labeling/logging)

## Naming: alternatives to `decomk.*`

The term “context” is core to the algorithm, but “contexts” is awkward
in filenames and hard to say. Alternatives for the `hosts.conf` analog:

Candidates:
- `profiles.conf` / `profiles.d/` (intuitive: “DEFAULT profile + repo profile”)
- `presets.conf` / `presets.d/` (also intuitive; implies “known-good defaults”)
- `blueprints.conf` / `blueprints.d/` (evokes “workspace blueprint”, but longer)
- `rules.conf` / `rules.d/` (generic; doesn’t convey composition as well)
- `macros.conf` / `macros.d/` (honest about the expansion mechanism; a bit jargon-y)
- Tool-namespaced: `decomk.conf` / `decomk.d/` (avoids bikeshedding; contents still define “keys”)

Decision: keep context definitions in `decomk.conf` (and `decomk.d/*.conf`)

## isconf mapping (what we’re borrowing)

### `platform` tool + `PLATFORM` variable
In isconf, `bin/platform` is executed early to set `PLATFORM`, `OS`, `OSVERSION`, and `HARDWARE`, which then:
- selects platform-specific defaults (e.g., which `make` binary to use)
- selects the included makefile (`conf/$(OS).mk`) from `conf/main.mk`

For Codespaces, OS is usually “Linux in a devcontainer”, but the same idea still helps:
- `PLATFORM=codespaces` (or `codespaces-ubuntu`) can gate “container-only” steps (apt installs, devcontainer assumptions)
- local runs can set `PLATFORM=local-darwin`/`local-linux` if we want parity
  - Decision (MVP): keep `PLATFORM` as a wrapper-set variable (no
    separate `platform` tool). Default to `PLATFORM=codespaces` when
    running in Codespaces; otherwise `PLATFORM=devcontainer`.

### `hosts.conf` and generated `etc/environment`
In isconf, `conf/hosts.conf` is the **source of truth** mapping a context key (e.g., `DEFAULT`, `HOSTNAME`) to:
- make targets (“packages”) and
- variable tuples (`KEY=value`)

Then `bin/mk_env` writes `$ISCONFDIR/etc/environment` as a **resolved snapshot** of the tuples for the chosen context. Make recipes may source it later (example: `conf/aix.mk` sources `$(ISCONFDIR)/etc/environment` before running a nested make).

Note: despite the name, this is *not* the host OS `/etc/environment`; it’s a generated file under the isconf tree’s `etc/` directory.

So: `hosts.conf` -> (expand macros) -> (select context) -> `etc/environment` snapshot. They are directly related.

## Devcontainer design (proposed)

### 1) A `decomk.conf` file (hosts.conf analog)
Add a `decomk.conf` file with the same “macro expansion” semantics as
isconf, but intended for devcontainers. Prefer the canonical
`decomk.conf` to live in the decomk config repo (so it can be shared
across workspaces), with an optional explicit `-config` override for
experimentation/overrides (which can point at a repo-local file):
- Decision: call it `decomk.conf` (and optionally `decomk.d/*.conf`)
  rather than `repos.conf`. The file will include `DEFAULT` and other
  non-repo keys, so “repos” is misleading.
  - Simplification option: skip a separate config file and encode
    per-repo defaults directly in the Makefile (e.g., by including
    `mk/contexts.mk`).
    - Pros: fewer moving parts; “just Make”.
    - Cons: harder to parse/expand safely; mixes policy with recipes;
      less portable if we later want a non-Make consumer (CLI/UI).

Example (conceptual):
- `DEFAULT: PLATFORM=codespaces Block00_base Block10_common`
- `grokker: DEFAULT Block20_go Block30_storm`
- `mob-consensus: DEFAULT Block20_go`

Decision: use isconf/lunamake-style `BlockNN_*` target groups for
phases/capabilities. Don’t introduce an additional namespace prefix
like `codespace.` or `cs.` unless we have a concrete collision problem.

The wrapper expands `DEFAULT` + `<repo>` into an argv list containing both `VAR=value` tuples and Make targets.

### 2) A wrapper around `make` (no Perl)
The wrapper resolves context and expansions, writes an env export file, then runs `make`.

Two implementation options:

**Option A: Bash wrapper**
- Pros: smallest bootstrap dependency; easy to run in early container lifecycle.
- Cons: robust recursive macro expansion + tokenization is fiddly; harder to unit test; quoting edge-cases.

**Option B: Go wrapper**
- Pros: clean implementation of the isconf `expandmacro` algorithm;
  easier tests; clearer error messages; safer parsing.
- Cons: requires Go toolchain in the container (or shipping a prebuilt
  binary); adds build/distribution decisions.
  - Decision: don’t commit binaries. For MVP, prefer `go run
    ./cmd/decomk ...` (no install step) or `go install ./cmd/decomk`
    during `postCreateCommand`. A “curl a release binary” approach can
    come later once the tool stabilizes.

`postCreateCommand` note: in `devcontainer.json`, `postCreateCommand`
runs after the container is created and the workspace is available.
It’s a natural place to run bootstraps like `decomk`.

Recommendation: start with **Go** if “correct expansion + testability” matters more than “no toolchain assumptions”. If Codespaces images differ per repo, a tiny **Bash** wrapper may be easier to guarantee runs everywhere.

### 3) An env export file (etc/environment analog)
Write a generated file containing the resolved tuples (plus a small set of computed `DECOMK_*` exports) for later sourcing by shell scripts or nested make invocations.

Choices:
- Container-local (recommended for devcontainers): `<DECOMK_HOME>/env.sh` (default `/var/decomk/env.sh`)

## Target groups (BLOCK_* analogs)

In isconf, “BlockXX” targets group packages into phases (bootstrap → base → tools). For Codespaces, similar grouping helps keep installs understandable and incremental.

Candidates:
- Phase-style groups: `Block00_base`, `Block10_lang`, `Block20_tools`, `Block30_editors`, `Block40_project`
- Capability groups: `Block20_go`, `Block21_node`, `Block30_neovim`, `Block30_llm`, `Block30_storm`
- Repo groups: avoid encoding the repo name into the target name; prefer
  expressing repo-specific composition in `decomk.conf` itself.

Pragmatic MVP: define a small set of **capability groups**, then compose per-repo contexts from them via `DEFAULT` + `<repo>`.

## Subtasks

- [x] 001.1 Decide naming for the `hosts.conf` analog (`decomk.conf` + optional `decomk.d/*.conf`).
- [x] 001.2 Choose config file name/location and syntax (use isconf-like `decomk.conf` grammar).
- [x] 001.3 Choose wrapper language (Go vs Bash) and document the tradeoffs/decision.
- [x] 001.4 Implement macro expansion (isconf `expandmacro` semantics) without Perl.
- [x] 001.5 Implement env export file generation and decide where it is written.
- [ ] 001.6 Define initial target groups (BLOCK_XX analogs) and a minimal `DEFAULT` toolset.
- [ ] 001.7 Define the update/self-update model: install-first (`go install`) for the tool binary with optional clone mode, plus config repo pull into `<DECOMK_HOME>/conf`; support a pinned config ref/branch (lunamake test→prod style).
- [ ] 001.8 Pilot in `mob-sandbox` via `devcontainer.json` `postCreateCommand`, then generalize.
- [x] 001.9 Add a reference `examples/devcontainer/postCreateCommand.sh` that performs stage-0 bootstrap (ensure decomk in `PATH`, sync config repo, run decomk), then reuse it in pilots.
- [x] 001.10 Add `decomk init` to scaffold `.devcontainer` files from embedded templates with flag/prompt inputs.
- [x] 001.11 Generate example scaffold files from canonical templates and enforce sync via `go generate`/tests.
- [x] 001.12 Make the production example devcontainer standalone by including a Dockerfile build entry and companion Dockerfile.
