# TODO 001 - decomk: isconf-like devcontainer tool bootstrap

## Decision Intent Log

ID: DI-001-20260424-211213
Date: 2026-04-24 21:12:13
Status: active
Decision: Add a stage-0 Go toolchain preflight after root escalation and use the resolved absolute Go binary path for all `go env`/`go install` calls.
Intent: Make root-escalated stage-0 failures explicit and deterministic when sudo path policy drops Go from PATH.
Constraints: Keep stage-0 non-interactive and fail-fast; preserve existing URI-driven install semantics; emit actionable error messages that include PATH context.
Affects: `cmd/decomk/templates/decomk-stage0.sh.tmpl`, generated stage-0 scripts, stage-0 template tests.

ID: DI-001-20260424-200248
Date: 2026-04-24 20:02:48
Status: active
Decision: Move privilege escalation to stage-0 startup (`sudo -n -E` self-exec), remove `decomk run` sudo/make-as-root controls, and require root for `decomk run`.
Intent: Keep root execution deterministic with one escalation boundary at stage-0 entry so make always runs as root without scattered sudo logic.
Constraints: Stage-0 must fail fast when escalation is unavailable; `decomk run` must emit a clear non-root error; remove `-make-as-root` and `DECOMK_MAKE_AS_ROOT` from code/docs/tests.
Affects: `cmd/decomk/templates/decomk-stage0.sh.tmpl`, `cmd/decomk/main.go`, `cmd/decomk/main_test.go`, `README.md`, selftest harness scripts and fixture Makefile.
Supersedes: DI-001-20260309-172358, DI-001-20260422-130000

ID: DI-001-20260424-193612
Date: 2026-04-24 19:36:12
Status: active
Decision: Use the same default devcontainer name resolution for `decomk init -conf` as consumer `decomk init`: repo root basename when `-name` is not set and no existing value is reused.
Intent: Eliminate mode-specific naming surprises so interactive defaults and non-interactive output are consistent across producer and consumer init flows.
Constraints: Preserve existing precedence (`-name` override first, then existing devcontainer defaults on `-f`, then basename fallback), and keep confrepo generator/example naming behavior unchanged.
Affects: `cmd/decomk/init.go`, `cmd/decomk/init_conf_test.go`, `README.md`.

ID: DI-001-20260424-190437
Date: 2026-04-24 19:04:37
Status: active
Decision: Unify repo bootstrap flows under `decomk init` with `-conf` producer mode, remove separate `init-conf` command, and make consumer identity (`DECOMK_DEV_USER`/`DECOMK_DEV_UID`) conf-driven by cloning the producer conf repo and reading `.devcontainer/devcontainer.json`.
Intent: Keep end-to-end identity deterministic across producer and consumer repos without duplicated init codepaths or per-repo manual user/UID edits.
Constraints: Consumer `decomk init` must fail fast when conf repo identity cannot be fetched or validated; producer mode must write identity metadata into devcontainer env plus runtime user fields; `updateRemoteUserUID` must be explicitly false in generated outputs.
Affects: `cmd/decomk/main.go`, `cmd/decomk/init.go`, `cmd/decomk/init_conf.go`, `stage0/stage0.go`, `confrepo/confrepo.go`, `cmd/decomk/templates/*`, generated examples, `README.md`, `doc/decomk-design.md`, `TODO/013-conf-repo-init-scaffolding.md`.
Supersedes: DI-001-20260311-161825 (command shape only), DI-001-20260425-005155 (identity source/flow details)

ID: DI-001-20260425-005155
Date: 2026-04-25 00:51:55
Status: active
Decision: Standardize decomk-owned test/codespaces devcontainer identities on username `dev` with UID `1000`, and encode that explicitly in generated devcontainer/Dockerfile scaffolds (`remoteUser`/`containerUser` + no UID remap).
Intent: Eliminate user-identity drift (`dev` vs `vscode` vs `codespace`) so stage-0, make, and harness assertions are deterministic across DevPod, devcontainer CLI, and Codespaces.
Constraints: Keep this contract limited to decomk-owned test/codespaces scaffolds and producer examples; do not force arbitrary existing user repos initialized with image-only configs to a potentially missing username.
Affects: `stage0/stage0.go`, `cmd/decomk/templates/devcontainer.json.tmpl`, `confrepo/confrepo.go`, `cmd/decomk/templates/confrepo.*.tmpl`, generated examples under `examples/*`, selftest harness assertions, and related tests/docs.

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

ID: DI-001-20260313-224000
Date: 2026-03-13 22:40:00
Status: active
Decision: Treat env export as a first-class runtime contract by resolving `NAME=$` passthrough tuples against incoming environment values (with prior-tuple fallback and fail-fast when unresolved), auto-carrying incoming `DECOMK_*` vars, and generating both `env.sh` and make invocation variables from one canonical tuple sequence.
Intent: Make `env.sh` authoritative (not debug-only) and guarantee that make plus child processes receive the same effective environment values even when make runs under sudo with environment reset.
Constraints: Keep config grammar unchanged (tuple syntax only), reserve literal tuple value `$` for passthrough semantics, and defer `env.mk` as an optional future derived artifact rather than the source of truth.
Affects: `cmd/decomk/main.go`, `cmd/decomk/main_test.go`, `README.md`, `doc/decomk-design.md`, `examples/decomk-selftest/fixtures/confrepo/scripts/verify_tool_repo.sh`.

ID: DI-001-20260313-174538
Date: 2026-03-13 17:45:38
Status: active
Decision: Keep tuple-driven `PATH` behavior in the canonical cooked env contract and do not split launcher-path semantics from make/recipe env semantics.
Intent: Preserve one explicit and predictable contract (`env.sh`/cooked tuples) instead of introducing a dual-model where launcher lookup uses one PATH while make/recipes use another.
Constraints: `PATH` tuple values may cause launcher failures when misconfigured, and that remains user-visible by design; defer any optional separate launcher-path mode to future design work if needed.
Affects: `cmd/decomk/main.go`, `README.md`.

ID: DI-001-20260412-170500
Date: 2026-04-12 17:05:00
Status: active
Decision: Replace stage-0 tool/config source inputs with URI expressions (`DECOMK_TOOL_URI`, `DECOMK_CONF_URI`) and update `decomk init` to scaffold only URI-based devcontainer values.
Intent: Remove mode/repo/install-package split configuration, keep stage-0 source selection explicit (`go:` or `git:`), and make templates/examples/init generation share one deterministic contract.
Constraints: Tool URI must accept `go:` and `git:` forms; config URI must accept only `git:`; generated scaffolds and embedded templates must stay synchronized.
Affects: `stage0/stage0.go`, `cmd/decomk/init.go`, `cmd/decomk/init_test.go`, `cmd/decomk/templates/*`, generated `examples/devcontainer/*`, generated selftest workspace templates, `README.md`.
Supersedes: DI-001-20260311-163942, DI-001-20260311-175002

ID: DI-001-20260412-194342
Date: 2026-04-12 19:43:42
Status: active
Decision: Make `decomk init` refuse to write when either target stage-0 file already exists unless `-force`/`-f` is set; when refusing, do not emit alternate files and instead print a commit/force/difftool reconciliation workflow. Also add non-edit banners to stage-0 templates and reorganize README around decomk philosophy + shared-conf onboarding.
Intent: Keep stage-0 adoption safe by default (no implicit overwrite), force explicit user intent for replacement, and provide one clear reconciliation path while making docs and generated file ownership unambiguous.
Constraints: Existing files must remain untouched in non-force mode even when one file is missing; no temp merge artifacts; `-f` must alias `-force`; README must link (not duplicate) legacy variable migration mapping.
Affects: `cmd/decomk/init.go`, `cmd/decomk/init_test.go`, `cmd/decomk/templates/devcontainer.json.tmpl`, `cmd/decomk/templates/postCreateCommand.sh.tmpl`, generated `examples/devcontainer/*`, generated selftest workspace templates, `README.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`.
Supersedes: DI-001-20260311-161825

ID: DI-001-20260416-222700
Date: 2026-04-16 22:27:00
Status: superseded
Decision: Split stage-0 lifecycle into two explicit entrypoints by generating `updateContentCommand` and `postCreateCommand` hooks that both call `.devcontainer/decomk-stage0.sh` with phase arguments (`update-content` and `post-create` respectively).
Intent: Align generated devcontainer contracts with measured Codespaces lifecycle behavior so common/prebuild work and runtime/user work can be reasoned about and tested separately.
Constraints: Keep one canonical stage-0 template source, keep generated examples/selftests in sync via stage0gen, and preserve explicit, non-silent bootstrap failures.
Affects: `cmd/decomk/templates/devcontainer.json.tmpl`, `cmd/decomk/templates/decomk-stage0.sh.tmpl`, `stage0/stage0.go`, `cmd/decomk/init*.go`, `cmd/stage0gen/main.go`, generated `examples/devcontainer/*`, generated selftest workspace templates, `examples/decomk-selftest/*`.

ID: DI-001-20260416-223600
Date: 2026-04-16 22:36:00
Status: active
Decision: Use camelCase phase arguments in stage-0 hook invocations (`updateContent`, `postCreate`) instead of kebab-case.
Intent: Keep lifecycle hook argument naming aligned with devcontainer hook key names (`updateContentCommand`, `postCreateCommand`) to reduce cognitive load.
Constraints: Preserve two-phase lifecycle behavior, keep `DECOMK_STAGE0_PHASE` values consistent with hook arguments, and update selftest markers/docs accordingly.
Affects: `stage0/stage0.go`, `cmd/decomk/templates/*`, generated `examples/devcontainer/*`, generated selftest workspace templates, `examples/decomk-selftest/*`, `README.md`, `doc/decomk-design.md`.
Supersedes: DI-001-20260416-222700

ID: DI-001-20260420-161700
Date: 2026-04-20 16:17:00
Status: active
Decision: Remove TODO subtasks `001.6` and `001.7` from TODO 001 because they are no longer valid work items.
Intent: Keep TODO 001 aligned with current scope and avoid carrying obsolete planning items that create false backlog.
Constraints: Preserve stable TODO numbering for remaining items and avoid renumbering other TODO entries.
Affects: `TODO/001-decomk-devcontainer-tool-bootstrap.md`.

ID: DI-001-20260421-221834
Date: 2026-04-21 22:18:34
Status: active
Decision: Track TODO `001.8` execution details in a dedicated cross-repo checklist file (`TODO/012-mob-sandbox-pilot.md`) instead of embedding those details in TODO 001.
Intent: Keep TODO 001 focused on decomk bootstrap scope while giving the mob-sandbox pilot a decision-complete, repo-scoped execution checklist.
Constraints: Keep TODO numbering stable, keep `001.8` open until TODO 012 completion criteria and evidence are satisfied, and label execution tasks by repository.
Affects: `TODO/001-decomk-devcontainer-tool-bootstrap.md`, `TODO/012-mob-sandbox-pilot.md`, `TODO/TODO.md`.

ID: DI-001-20260422-110600
Date: 2026-04-22 11:06:00
Status: active
Decision: Track shared config repo scaffolding as a dedicated follow-on workstream (`TODO/013-conf-repo-init-scaffolding.md`) and reference it from TODO 001 via a new subtask.
Intent: Keep TODO 001 aligned with the expanded bootstrap contract where conf repos can be initialized with decomk, without overloading existing subtask scope.
Constraints: Preserve stable TODO numbering and keep the conf-repo-init implementation details centralized in TODO 013.
Affects: `TODO/001-decomk-devcontainer-tool-bootstrap.md`, `TODO/013-conf-repo-init-scaffolding.md`, `TODO/TODO.md`.

ID: DI-001-20260422-130000
Date: 2026-04-22 13:00:00
Status: active
Decision: In root-make mode, make `resolveMakeCommand` prefer `sudo --preserve-env=PATH,GITHUB_USER`, fall back to `PATH`-only preserve, then plain `sudo`, while keeping dry-run and non-root modes unchanged.
Intent: Keep postCreate user-phase checks and Makefile behavior consistent when make is launched through sudo, without introducing hard failures on restrictive sudo policies.
Constraints: Preserve existing passwordless-sudo requirements, avoid broad env preservation beyond `PATH` + `GITHUB_USER`, and keep fallback behavior explicit/tested.
Affects: `cmd/decomk/main.go`, `cmd/decomk/main_test.go`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`.

ID: DI-001-20260423-045924
Date: 2026-04-23 04:59:24
Status: active
Decision: Add a first-class `decomk version` subcommand that prints one CLI version string and rejects positional args.
Intent: Give operators and automation a stable, script-friendly way to identify decomk binary version without parsing help text.
Constraints: Default version string stays `dev` unless overridden at build time; command output remains a single line.
Affects: `cmd/decomk/main.go`, `cmd/decomk/*_test.go`, `README.md`.

ID: DI-001-20260423-051500
Date: 2026-04-23 05:15:00
Status: active
Decision: Make `decomk init` refuse existing stage-0 targets before any interactive prompts, and change the canonical default `DECOMK_TOOL_URI` from `@stable` to `@latest`.
Intent: Prevent unnecessary prompt churn when overwrite would be refused anyway, and align default tool bootstrap behavior with newest release by default.
Constraints: Preserve existing non-force overwrite guidance text and `-f/-force` override behavior; keep explicit `DECOMK_TOOL_URI` values unchanged.
Affects: `cmd/decomk/init.go`, `cmd/decomk/init_test.go`, `stage0/stage0.go`, `cmd/decomk/templates/decomk-stage0.sh.tmpl`, generated `examples/*/.devcontainer/*`, `README.md`.

ID: DI-001-20260423-204251
Date: 2026-04-23 20:42:51
Status: active
Decision: Make `VERSION` the canonical source for CLI release version, generate `cmd/decomk/version_generated.go` from it via `go generate`, add `make release-minor` backed by `scripts/release.sh` to bump minor version + commit + tag + push, and have `decomk version` default to the generated value.
Intent: Keep release versioning deterministic and source-controlled so runtime `decomk version` matches release tags without depending on external binary publishing workflows.
Constraints: Release script must fail on dirty repos, must refuse duplicate tags, and must update VERSION plus generated version source in one release commit before tagging.
Affects: `VERSION`, `cmd/versiongen/main.go`, `cmd/decomk/generate_version.go`, `cmd/decomk/version_generated.go`, `cmd/decomk/main.go`, `scripts/release.sh`, `Makefile`, `README.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`.

ID: DI-001-20260423-140628
Date: 2026-04-23 14:06:28
Status: active
Decision: Add `image` rendering support to the stage-0 devcontainer template (defaulting to a canonical base image when no build dockerfile is set), and make `decomk init` reuse values from an existing `.devcontainer/devcontainer.json` as defaults when rerunning with `-f`.
Intent: Keep non-Dockerfile devcontainers valid out of the box while reducing friction and accidental config drift during forced re-init runs.
Constraints: Preserve non-force overwrite refusal behavior; keep existing build-backed devcontainers in build mode when rerunning `decomk init -f` unless `-image` is explicitly provided.
Affects: `stage0/stage0.go`, `cmd/decomk/templates/devcontainer.json.tmpl`, `cmd/decomk/init.go`, `cmd/decomk/init_test.go`, `README.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`.

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
- [ ] 001.8 Execute the `mob-sandbox` pilot and generalize outcomes, tracked in `TODO/012-mob-sandbox-pilot.md`.
- [x] 001.9 Add a reference `examples/devcontainer/postCreateCommand.sh` that performs stage-0 bootstrap (ensure decomk in `PATH`, sync config repo, run decomk), then reuse it in pilots.
- [x] 001.10 Add `decomk init` to scaffold `.devcontainer` files from embedded templates with flag/prompt inputs.
- [x] 001.11 Generate example scaffold files from canonical templates and enforce sync via `go generate`/tests.
- [x] 001.12 Make the production example devcontainer standalone by including a Dockerfile build entry and companion Dockerfile.
- [ ] 001.13 Add optional derived `env.mk` output generated from the same canonical env tuple sequence as `env.sh` (without making `env.mk` the source of truth).
- [x] 001.14 Add strict non-overwrite defaults to `decomk init` (fail unless `-f`/`-force` when target files exist) and provide explicit commit/force/difftool reconciliation guidance.
- [x] 001.15 Add stage-0 template ownership banners and README onboarding reorganization, plus a canonical legacy-variable migration mapping section.
- [x] 001.16 Add first-class shared conf repo scaffolding (`decomk init -conf`), tracked in `TODO/013-conf-repo-init-scaffolding.md`.
- [x] 001.17 Add source-controlled release versioning (`VERSION` + generated version file) and a `make release-minor` workflow.
- [x] 001.18 Add image fallback rendering for non-build init scaffolds and reuse existing devcontainer values as defaults during `decomk init -f`.

## Legacy stage-0 variable migration mapping

The canonical stage-0 contract is URI-based (`DECOMK_TOOL_URI`, `DECOMK_CONF_URI`).
If you are migrating from older examples, use this mapping:

| Legacy variable/flag | New variable/flag | Notes |
| --- | --- | --- |
| `DECOMK_CONF_REPO` / `-conf-repo` | `DECOMK_CONF_URI` / `-conf-uri` | Use `git:<repo-url>[?ref=<git-ref>]`. |
| `DECOMK_TOOL_REPO` + `DECOMK_TOOL_MODE=clone` | `DECOMK_TOOL_URI=git:<repo-url>[?ref=<git-ref>]` | Clone/pull + `go install ./cmd/decomk` behavior is selected by `git:` URI. |
| `DECOMK_TOOL_INSTALL_PKG` + `DECOMK_TOOL_MODE=install` | `DECOMK_TOOL_URI=go:<module>@<version>` | Install-first behavior is selected by `go:` URI. |
