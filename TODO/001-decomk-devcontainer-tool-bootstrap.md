# TODO 001 - decomk: isconf-like devcontainer tool bootstrap

Goal: create an isconf-inspired “context -> target groups + vars”
bootstrap so a GitHub Codespace can automatically install **shared**
dev tools plus **repo-specific** tools using a Makefile, without
needing hostname-based host configuration or Perl.

## Desired properties

- Deterministic: the same repo creates the same toolset by default.
- Layered: `DEFAULT` (shared tools) + `<repo>` (repo-specific tools).
- Idempotent: re-running is safe (good for `postCreateCommand` +
  `postStartCommand`).
    - XXX see the wikipedia idempotent page for more nuanced
      properties (e.g., “side effects” like package installations may
      not be idempotent, but the resulting state should be).  also see
      the monoid wikipedia page.
    - Working definition for this tool: repeated runs should converge
      on the same declared toolset without breaking a working
      environment. Side effects are expected (and are the point) and
      OK if they are safe on re-run (stamps prevent re-runs of those
      that aren't safe).
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

Implication for `spaceconf` (this TODO’s tool):
- We should treat **Dev Container** as the portable hook point
  (`postCreateCommand` runs `spaceconf make ...`), and treat Codespaces
  as *one* platform that provides useful default identity (`owner/repo`,
  codespace name, etc.).
- For non-Codespaces runs (local devcontainers, DevPod, GCP self-host),
  require an explicit override env var (`SPACECONF_CONTEXT`, etc.) so we
  don’t bake GitHub-specific assumptions into the core algorithm.

## Context identity (Codespaces)

Inputs (preferred order):
1. `GITHUB_REPOSITORY` (required in Codespaces; `owner/repo`)  --
   everything after the slash is the repo name, which is a natural
   context key. BUT if there is a makefile target for the full
   owner/repo, that should take precedence over the repo name alone
   (e.g., `stevegt/grokker` should take precedence over `grokker` if
   both exist). 
   - XXX do we handle this in e.g. repos.conf, or just in the wrapper
     logic?
   - Recommendation: handle precedence in the wrapper by trying a list
     of candidate context keys in order (override var → `owner/repo` →
     `repo` → `DEFAULT`); the config file can simply define whichever
     keys are needed.
2. Optional override: `CSWG_CONTEXT` / `CSWG_REPO` (for local dev and
tests)
   - XXX we need a name for this tool, not "CSWG". something like:
    - spacemaker: it "makes" the "space" (Codespace) ready to work
    - spacemakr: more unique
    - spaceconf: like isconf but for Codespaces
    - spaceconfig: more unique but less catchy
   - Recommendation: use `spaceconf` as a working name (binary name,
     config naming, env var prefix: `SPACECONF_*`). If we want to
     avoid churn, support `CSWG_*` as deprecated aliases during
     transition.

Derived:
- `REPO_NAME=${GITHUB_REPOSITORY##*/}`
- `CODESPACE_NAME=$CODESPACE_NAME` (optional; useful for labeling/logging)

## isconf mapping (what we’re borrowing)

### `platform` tool + `PLATFORM` variable
In isconf, `bin/platform` is executed early to set `PLATFORM`, `OS`, `OSVERSION`, and `HARDWARE`, which then:
- selects platform-specific defaults (e.g., which `make` binary to use)
- selects the included makefile (`conf/$(OS).mk`) from `conf/main.mk`

For Codespaces, OS is usually “Linux in a devcontainer”, but the same idea still helps:
- `PLATFORM=codespaces` (or `codespaces-ubuntu`) can gate “container-only” steps (apt installs, devcontainer assumptions)
- local runs can set `PLATFORM=local-darwin`/`local-linux` if we want parity
- XXX not sure we need platform
  - Recommendation: for an MVP that targets Codespaces only, hardcode
    `PLATFORM=codespaces` in the wrapper and keep the variable as a
    future extension point (no separate `platform` tool needed yet).

### `hosts.conf` and generated `etc/environment`
In isconf, `conf/hosts.conf` is the **source of truth** mapping a context key (e.g., `DEFAULT`, `HOSTNAME`) to:
- make targets (“packages”) and
- variable tuples (`KEY=value`)

Then `bin/mk_env` writes `$ISCONFDIR/etc/environment` as a **resolved snapshot** of the tuples for the chosen context. Make recipes may source it later (example: `conf/aix.mk` sources `$(ISCONFDIR)/etc/environment` before running a nested make).

Note: despite the name, this is *not* the host OS `/etc/environment`; it’s a generated file under the isconf tree’s `etc/` directory.

So: `hosts.conf` -> (expand macros) -> (select context) -> `etc/environment` snapshot. They are directly related.

## Codespaces design (proposed)

### 1) A `contexts.conf` file (hosts.conf analog)
Add a repo-local file (name TBD) with the same “macro expansion” semantics as isconf, but intended for Codespaces:
- XXX call this repos.conf?  pros and cons.
  - Recommendation: prefer `contexts.conf` (or `spaceconf.conf`) over
    `repos.conf`. The file will include `DEFAULT` and other non-repo
    contexts, so “repos” is a little misleading. If we later want
    owner/repo vs repo-only keys, “context” still fits.
  - Simplification option: skip a separate config file and encode
    per-repo defaults directly in the Makefile (e.g., by including
    `mk/contexts.mk`).
    - Pros: fewer moving parts; “just Make”.
    - Cons: harder to parse/expand safely; mixes policy with recipes;
      less portable if we later want a non-Make consumer (CLI/UI).

Example (conceptual):
- `DEFAULT: PLATFORM=codespaces codespace.base codespace.common`
- `grokker: DEFAULT codespace.go codespace.storm`
- `mob-consensus: DEFAULT codespace.go`
- XXX codespace. in front of everything is ugly
  - Recommendation: use a shorter prefix like `cs.` (e.g., `cs.base`,
    `cs.common`, `cs.go`, `cs.storm`) or `space.` if we want to avoid
    ambiguity. Keep names noun-like so they compose cleanly.
    - XXX why have the prefix at all?

The wrapper expands `DEFAULT` + `<repo>` into an argv list containing both `VAR=value` tuples and Make targets.

### 2) A wrapper around `make` (no Perl)
The wrapper resolves context and expansions, writes an env snapshot, then runs `make`.

Two implementation options:

**Option A: Bash wrapper**
- Pros: smallest bootstrap dependency; easy to run in early container lifecycle.
- Cons: robust recursive macro expansion + tokenization is fiddly; harder to unit test; quoting edge-cases.

**Option B: Go wrapper**
- Pros: clean implementation of the isconf `expandmacro` algorithm;
  easier tests; clearer error messages; safer parsing.
- Cons: requires Go toolchain in the container (or shipping a prebuilt
  binary); adds build/distribution decisions.
- XXX we can ship a prebuilt binary by using curl in the Dockerfile,
  or we can add a `go install` step to the `postCreateCommand` (which
  is more self-contained but less efficient for rebuilds).  
  - Recommendation: don’t commit binaries. For MVP, prefer `go run
    ./cmd/spaceconf ...` (no install step) or `go install ./cmd/spaceconf`
    during `postCreateCommand`. A “curl a release binary” approach can
    come later once the tool stabilizes.
    - XXX what is postCreateCommand?

Recommendation: start with **Go** if “correct expansion + testability” matters more than “no toolchain assumptions”. If Codespaces images differ per repo, a tiny **Bash** wrapper may be easier to guarantee runs everywhere.

### 3) An env snapshot file (etc/environment analog)
Write a generated file containing the resolved tuples for the chosen context, for later sourcing by shell scripts or nested make invocations.

Choices:
- Repo-local (isconf-like): `etc/environment` or `etc/codespace.env`
- User-local: `~/.config/cswg/environment` (persists across rebuilds but less auditable per repo)

## Target groups (BLOCK_* analogs)

In isconf, “BlockXX” targets group packages into phases (bootstrap → base → tools). For Codespaces, similar grouping helps keep installs understandable and incremental.

Candidates:
- Phase-style groups: `cs00.base`, `cs10.lang`, `cs20.tools`, `cs30.editors`, `cs40.project`
- Capability groups: `codespace.base`, `codespace.go`, `codespace.node`, `codespace.neovim`, `codespace.llm`, `codespace.storm`
- Repo groups: `codespace.repo.grokker`, `codespace.repo.mob-consensus`, etc.
- XXX the “codespace.” prefix is a bit too verbose
  - Recommendation: if we keep a prefix at all, shorten to `cs.` and
    reserve `cs.repo.*` only for rare repo-specific phases (most repos
    should compose capability groups instead).
    - XXX why do we need a prefix?

Pragmatic MVP: define a small set of **capability groups**, then compose per-repo contexts from them via `DEFAULT` + `<repo>`.

## Subtasks

- [ ] 013.1 Decide where the tool lives (per-repo vs shared repo imported into each devcontainer).
- [ ] 013.2 Choose config file name/location and syntax (`contexts.conf` analog of `hosts.conf`).
- [ ] 013.3 Choose wrapper language (Go vs Bash) and document the tradeoffs/decision.
- [ ] 013.4 Implement macro expansion (isconf `expandmacro` semantics) without Perl.
- [ ] 013.5 Implement env snapshot generation (tuples-only) and decide where it is written.
- [ ] 013.6 Define initial Codespaces target groups (BLOCK_* analogs) and a minimal `DEFAULT` toolset.
- [ ] 013.7 Pilot in one repo (e.g., `grokker`) via `devcontainer.json` `postCreateCommand`, then generalize.
