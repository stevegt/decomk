# TODO 007 - decomk: migrate from Codespaces to DevPod on self-hosted GCP

## Decision Intent Log

ID: DI-007-20260424-212300
Date: 2026-04-24 21:23:00
Status: active
Decision: Pin the Codespaces selftest base image to `mcr.microsoft.com/devcontainers/base:ubuntu-24.04` (not plain `ubuntu:24.04`) and continue explicit apt installation of required bootstrap/runtime tools.
Intent: Align the selftest environment with devcontainer-native base behavior while keeping toolchain availability deterministic for root stage-0 execution.
Constraints: Preserve non-root `dev` user identity (`uid 1000`) with passwordless sudo and keep workspace ownership unchanged.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, selftest environment assumptions.
Supersedes: DI-007-20260424-211213

ID: DI-007-20260424-211213
Date: 2026-04-24 21:12:13
Status: active
Decision: Switch Codespaces selftest base image from `golang:1.24-bookworm` to `ubuntu:24.04` and install required bootstrap/runtime tools with apt (`golang-go`, `make`, `git`, `sudo`, `ca-certificates`).
Intent: Keep codespaces selftest toolchain availability explicit and aligned with root stage-0 execution so bootstrap does not depend on inherited base-image PATH behavior.
Constraints: Preserve non-root `dev` user identity (`uid 1000`) with passwordless sudo and keep workspace ownership unchanged.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, `examples/decomk-selftest/codespaces/run.sh` failure analysis assumptions, related docs/tests.

ID: DI-007-20260309-114541
Date: 2026-03-09 11:45:41
Status: active
Decision: Standardize devcontainer execution on DevPod with a GCP machine provider, and migrate from Codespaces using a dual-run transition.
Intent: Keep the devcontainer contract intact while moving control, cost, and operational policy to self-hosted GCP infrastructure.
Constraints: Preserve decomk non-interactive behavior (no sudo prompts), preserve current make-as-root default behavior, and preserve `DECOMK_HOME` and `DECOMK_LOG_DIR` semantics.
Affects: `TODO/007-devpod-gcp-selfhost-migration.md`, `TODO/TODO.md`, future `examples/devpod/*`, future `infra/terraform/*`, `README.md`.

ID: DI-007-20260309-124345
Date: 2026-03-09 12:43:45
Status: active
Decision: Implement an automated self-test harness for local DevPod with Docker provider under `examples/decomk-selftest`, make Codespaces parity checks the next stage, and defer remote GCP-provider test automation until a later move-to-GCP decision.
Intent: Lock down decomk bootstrap behavior in the environment we can validate today, while avoiding premature coupling to remote-provider decisions that are not yet approved.
Constraints: Keep tests non-interactive, preserve current decomk runtime behavior, avoid repository file mutation during test runs, and keep GCP validation out of the immediate harness scope.
Affects: `examples/decomk-selftest/*`, `TODO/007-devpod-gcp-selfhost-migration.md`, `README.md`.
Supersedes: DI-007-20260309-114541 (validation harness scope only)

ID: DI-007-20260309-180029
Date: 2026-03-09 18:00:29
Status: active
Decision: Refactor the local DevPod self-test harness so `decomk run SELFTEST_*` drives scenario execution from fixture `decomk.conf` + `Makefile` + scripts, while the outer harness only provisions a workspace and reads result files.
Intent: Move scenario logic to decomk-native config/make layers (isconf-style) so self-test behavior is easier to reason about, easier to extend, and less duplicated across wrapper shell scripts.
Constraints: Run all selected subtests in one fresh container per harness invocation, preserve targeted subtest selection, keep fixtures deterministic/offline, and keep machine-readable pass/fail outputs for automation.
Affects: `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/fixtures/confrepo/*`, `examples/decomk-selftest/README.md`, `README.md`.
Supersedes: DI-007-20260309-124345 (harness orchestration implementation only)

ID: DI-007-20260309-193525
Date: 2026-03-09 19:35:25
Status: active
Decision: Make self-test selector strings identical across `run.sh --subtests`, `decomk run` action args, and fixture tuple names (with `all` as the aggregate selector).
Intent: Remove alias-map indirection between harness layers so selector semantics are easier to read, maintain, and extend without drift bugs.
Constraints: Preserve single-container execution model, keep aggregate + subset selection behavior, and keep result-file validation deterministic.
Affects: `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/fixtures/confrepo/decomk.conf`, `examples/decomk-selftest/fixtures/confrepo/Makefile`, `examples/decomk-selftest/README.md`, `README.md`.
Supersedes: DI-007-20260309-180029 (selector naming only)

ID: DI-007-20260309-222412
Date: 2026-03-09 22:24:12
Status: active
Decision: Make `run.sh` pass raw `decomk run` arguments into the container, use literal Makefile targets as the primary selectors, and validate executed scenarios via an in-container `executed.list` manifest instead of a selector-expansion table.
Intent: Keep self-test behavior aligned with isconf-style action resolution (literal + tuple + mixed + context tuple) while removing translation-table maintenance between harness layers.
Constraints: Preserve one-container-per-run execution, keep deterministic machine-readable pass/fail output, and keep temporary fixtures offline.
Affects: `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/fixtures/confrepo/decomk.conf`, `examples/decomk-selftest/fixtures/confrepo/Makefile`, `examples/decomk-selftest/fixtures/confrepo/scripts/*`, `examples/decomk-selftest/README.md`, `README.md`, `doc/decomk-design.md`.
Supersedes: DI-007-20260309-193525 (selector contract details)

ID: DI-007-20260311-101409
Date: 2026-03-11 10:14:09
Status: active
Decision: Remove harness compatibility flags and context overrides; make run.sh publish a temporary fixture config repo over git://, use a minimal production-identical postCreate hook that only sets `DECOMK_CONF_REPO` and runs decomk, and validate results by parsing container make logs for PASS/FAIL markers.
Intent: Ensure selftest exercises decomk’s real config clone/pull path and isconf-like automatic context selection by workspace identity while keeping lifecycle hooks simple and test logic entirely in config-repo make/scripts.
Constraints: No `--subtests`/`--scenario`/`-context` in run.sh, no postCreate test validation, no postCreate config-repo mutation beyond URL handoff, and explicit checks that context tuples override DEFAULT while unrelated DEFAULT tuples remain available.
Affects: `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`, `examples/devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/fixtures/confrepo/decomk.conf`, `examples/decomk-selftest/fixtures/confrepo/Makefile`, `examples/decomk-selftest/fixtures/confrepo/scripts/*`, `examples/decomk-selftest/README.md`, `README.md`, `doc/decomk-design.md`.
Supersedes: DI-007-20260309-222412 (selftest execution plumbing)

ID: DI-007-20260311-145221
Date: 2026-03-11 14:52:21
Status: active
Decision: Move decomk tool/config clone-pull responsibilities into generic stage-0 postCreate bootstrap, have run.sh serve both repos over git:// and inject `DECOMK_TOOL_REPO` + `DECOMK_CONF_REPO` directly, and remove decomk core self-update/config-pull side effects from `decomk run`.
Intent: Resolve the bootstrap chicken-and-egg problem cleanly, keep lifecycle behavior deterministic, and make selftest exercise the same stage-0 path expected in production devcontainers.
Constraints: postCreate remains generic and production-identical, run.sh keeps automatic context detection (no `-context`), and tuple-precedence checks continue to verify context-overrides-default while unrelated DEFAULT tuples stay available.
Affects: `cmd/decomk/main.go`, `examples/devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/postCreateCommand.sh`, `examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json`, `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/fixtures/confrepo/*`, `examples/decomk-selftest/README.md`, `README.md`, `doc/decomk-design.md`.
Supersedes: DI-007-20260311-101409 (stage-0 bootstrap mechanics)

ID: DI-007-20260313-101500
Date: 2026-03-13 10:15:00
Status: active
Decision: Remove `.PHONY` from idempotent selftest fixture targets and add explicit stamp regression checks (probe + verify) in the DevPod harness, plus a core `cmdRun` unit test that asserts make runs in `<DECOMK_HOME>/stamps`.
Intent: Ensure decomk’s stamp semantics are validated end-to-end and in core tests so idempotent targets are skipped on rerun and make working-directory assumptions stay enforced.
Constraints: Keep stage-0 bootstrap non-interactive and production-identical, keep selftest marker-based pass/fail parsing, and avoid requiring root privileges in unit tests.
Affects: `examples/decomk-selftest/fixtures/confrepo/Makefile`, `examples/decomk-selftest/fixtures/confrepo/decomk.conf`, `examples/decomk-selftest/devpod-local/run.sh`, `cmd/decomk/main_test.go`, `examples/decomk-selftest/README.md`.

ID: DI-007-20260313-181500
Date: 2026-03-13 18:15:00
Status: active
Decision: Keep the aggregate `all` selftest target explicitly phony and never stamp it, while retaining stamp semantics for idempotent leaf targets.
Intent: Ensure `all` always orchestrates the full check set and cannot be skipped due to stale aggregate stamp artifacts, without weakening idempotent stamp behavior for real work targets.
Constraints: Do not reintroduce `.PHONY` on idempotent leaf targets; keep marker-based harness validation unchanged.
Affects: `examples/decomk-selftest/fixtures/confrepo/Makefile`.

ID: DI-007-20260412-171000
Date: 2026-04-12 17:10:00
Status: active
Decision: Switch selftest stage-0 fixture wiring from repo URL vars to URI vars by serving both tool/conf fixtures over `git://` and injecting `DECOMK_TOOL_URI` + `DECOMK_CONF_URI` into generated devcontainer inputs.
Intent: Keep selftest aligned with the production URI-based stage-0 contract while preserving automatic context detection, fixture-only PASS/FAIL logic, and no scenario translation table.
Constraints: `run.sh` remains the only component that publishes fixture repos; postCreate remains generated/minimal and does not own test logic; verification scripts must parse `git:` URIs to assert origin URLs.
Affects: `examples/decomk-selftest/devpod-local/run.sh`, selftest fixture `Makefile` and scripts, generated selftest workspace template files, `examples/decomk-selftest/README.md`, `doc/decomk-design.md`.
Supersedes: DI-007-20260311-145221

ID: DI-007-20260412-230000
Date: 2026-04-12 23:00:00
Status: active
Decision: Add a Codespaces parity harness (`examples/decomk-selftest/codespaces/run.sh`) that creates a fresh Codespace from the pushed branch under test, injects stage-0 URI env vars, runs production stage-0 bootstrap script, validates fixture PASS/FAIL markers and stamp checks, and then tears down the Codespace by default.
Intent: Prove decomk stage-0 + run behavior parity between local DevPod and GitHub Codespaces while keeping test assertions fixture-driven and avoiding hidden harness-side scenario logic.
Constraints: Harness must fail fast when local `HEAD` is not pushed to `origin/<branch>`, use repo-hosted devcontainer config for create-time reproducibility, preserve diagnostics on failure, and support an explicit debug mode that keeps failed Codespaces alive.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `examples/decomk-selftest/codespaces/.devcontainer/*`, `examples/decomk-selftest/README.md`, `README.md`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260413-000500
Date: 2026-04-13 00:05:00
Status: active
Decision: Make Codespaces parity harness self-contained by defaulting config source to an auto-generated fixture git repo inside the Codespace, while retaining optional `--conf-uri` override for external fixture repos.
Intent: Remove unnecessary operator-provided fixture URI plumbing for normal runs and keep stage-0 coverage realistic by still cloning config from a URI.
Constraints: Keep pushed-branch requirement, keep marker/stamp assertions fixture-driven, preserve failure artifact collection, and avoid mutating the repository under test.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `examples/decomk-selftest/README.md`, `README.md`.
Supersedes: DI-007-20260412-230000 (config source requirement only)

ID: DI-007-20260413-011500
Date: 2026-04-13 01:15:00
Status: active
Decision: Move Codespaces harness devcontainer config under `.devcontainer/` for API compatibility, and default harness creates to a non-interactive machine type (`basicLinux32gb`) with an explicit `--machine` override.
Intent: Eliminate known `gh codespace create` failures from invalid config path and interactive machine prompts so parity runs stay automation-friendly.
Constraints: Preserve pushed-branch guardrails and existing marker/stamp validation flow; keep path override available but restricted to `.devcontainer/*` for compatibility.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `.devcontainer/codespaces-selftest/devcontainer.json`, `.devcontainer/codespaces-selftest/Dockerfile`, `TODO/007-devpod-gcp-selfhost-migration.md`.
Supersedes: DI-007-20260412-230000 (devcontainer path + machine selection details only)

ID: DI-007-20260413-014500
Date: 2026-04-13 01:45:00
Status: active
Decision: Auto-resolve Codespaces machine type from the account-allowed machine list when `--machine` is unset (preferring `basicLinux32gb` when available), while retaining explicit `--machine` override with validation.
Intent: Remove fragile hardcoded machine assumptions and prevent interactive `gh codespace create` prompts from breaking automation on accounts with different policy-allowed machine sets.
Constraints: Keep non-interactive create path, fail fast with actionable errors when no machine is available or an override is invalid, and preserve existing codespace lifecycle and marker/stamp validation behavior.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `examples/decomk-selftest/README.md`, `README.md`, `TODO/007-devpod-gcp-selfhost-migration.md`.
Supersedes: DI-007-20260413-011500 (machine defaulting behavior only)

ID: DI-007-20260413-040500
Date: 2026-04-13 04:05:00
Status: active
Decision: Resolve Codespaces machine choices from repo-scoped machine API first (`repos/<owner>/<repo>/codespaces/machines`), fall back to legacy user-scoped API when available, and fail fast instead of propagating API error payloads into `--machine`.
Intent: Prevent harness misconfiguration when Codespaces API shape/entitlements differ by account or repo, while preserving non-interactive machine selection and explicit `--machine` overrides.
Constraints: Keep deterministic preferred machine ordering (`basicLinux32gb` first when available), keep actionable error text for invalid overrides, and preserve existing marker/stamp test flow.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `README.md`, `TODO/007-devpod-gcp-selfhost-migration.md`.
Supersedes: DI-007-20260413-014500 (machine discovery mechanism only)

ID: DI-007-20260413-043500
Date: 2026-04-13 04:35:00
Status: active
Decision: Add progress output (dot ticks with periodic elapsed time) during Codespaces discovery and SSH readiness wait loops.
Intent: Improve operator feedback during slow startup so waits are visibly active and easier to distinguish from stalled harness execution.
Constraints: Keep wait-loop behavior unchanged, avoid contaminating command-substitution outputs, and preserve timeout-based failure semantics.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260413-051500
Date: 2026-04-13 05:15:00
Status: active
Decision: Require SSH server availability in the Codespaces selftest devcontainer by enabling `ghcr.io/devcontainers/features/sshd:1`, and surface an explicit harness error hint when SSH readiness does not come up.
Intent: Ensure harness remote execution via `gh codespace ssh` is reliable and failures are actionable when stale/badly configured codespaces are used.
Constraints: Keep the harness workflow unchanged, preserve timeout semantics, and avoid requiring manual per-run SSH setup.
Affects: `.devcontainer/codespaces-selftest/devcontainer.json`, `examples/decomk-selftest/codespaces/run.sh`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260413-053500
Date: 2026-04-13 05:35:00
Status: active
Decision: Reuse stage-0 environment roots (`DECOMK_HOME`, `DECOMK_LOG_DIR`) for all post-bootstrap `decomk run` checks in the Codespaces harness.
Intent: Keep stamp verification runs on the same config/stamp/log state initialized by stage-0 so harness checks do not silently fall back to `/var/decomk` defaults.
Constraints: Preserve existing marker-based assertions, keep postCreate behavior unchanged, and avoid harness-side config mutation between runs.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260412-042700
Date: 2026-04-12 04:27:00
Status: active
Decision: Keep local Codespaces harness artifacts under `/tmp/decomk-codespaces.*` by default after successful runs, with an explicit `--cleanup` flag to opt into removal.
Intent: Make successful parity runs inspectable without reruns while preserving an explicit cleanup path for operators who prefer ephemeral local artifacts.
Constraints: Do not alter failure artifact retention semantics, keep Codespace lifecycle behavior unchanged, and preserve existing marker/stamp validation flow.
Affects: `examples/decomk-selftest/codespaces/run.sh`, `examples/decomk-selftest/README.md`, `README.md`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260412-043200
Date: 2026-04-12 04:32:00
Status: active
Decision: Add root `make` wrappers for local DevPod and Codespaces selftests, including an explicit cleanup variant for Codespaces runs.
Intent: Make selftest execution muscle-memory simple and consistent so operators can run parity checks without remembering script paths and cleanup flags.
Constraints: Keep wrapper targets phony, preserve existing harness behavior, and keep cleanup optional rather than default.
Affects: `Makefile`, `README.md`, `examples/decomk-selftest/README.md`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260416-031409
Date: 2026-04-16 03:14:09
Status: active
Decision: Keep the near-term self-hosting plan on stock DevPod/Codespaces behavior, and defer any fork/wrapper work (DevPod or devcontainer reference CLI adaptation to emulate prebuild lifecycle hooks via build+up+snapshot) to a later migration phase.
Intent: Avoid expanding immediate migration risk while preserving a tracked path to evaluate custom runtime behavior only if hosted/self-hosted parity requires it.
Constraints: No immediate runtime forks; keep current phase focused on evidence gathering and baseline migration readiness.
Affects: `TODO/007-devpod-gcp-selfhost-migration.md`, `TODO/009-phase-eval-lifecycle-spike.md`, future `examples/phase-eval/*`.

ID: DI-007-20260417-135300
Date: 2026-04-17 13:53:00
Status: active
Decision: Track image freeze/parity implementation in dedicated TODO 011 and `doc/image-management.md`, while keeping TODO 007 focused on migration sequencing and provider/platform operations.
Intent: Separate migration orchestration from image-parity implementation details so each workstream has clear scope and acceptance gates.
Constraints: Preserve TODO 007 migration phases and deferred GCP decisions; reference TODO 011 as the source of truth for freeze/parity mechanics.
Affects: `TODO/007-devpod-gcp-selfhost-migration.md`, `TODO/011-single-path-checkpoints.md`, `doc/image-management.md`, `TODO/TODO.md`.

ID: DI-007-20260423-180202
Date: 2026-04-23 18:02:02
Status: active
Decision: Emit explicit stage-0 and make identity markers (`uid`/`user`) in selftest flows and assert them in both DevPod and Codespaces harnesses.
Intent: Provide concrete evidence about which user executes stage-0 vs make so root/non-root behavior questions can be answered from harness artifacts instead of inference.
Constraints: Keep markers deterministic and phase-labeled (`updateContent`/`postCreate`), preserve existing PASS/FAIL marker contract, and avoid introducing silent fallback logic.
Affects: `cmd/decomk/templates/decomk-stage0.sh.tmpl`, generated `examples/*/decomk-stage0.sh`, `examples/decomk-selftest/fixtures/confrepo/Makefile`, `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/codespaces/run.sh`, `cmd/decomk/stage0_script_test.go`, `TODO/007-devpod-gcp-selfhost-migration.md`.

ID: DI-007-20260424-200248
Date: 2026-04-24 20:02:48
Status: active
Decision: Update selftest identity assertions to require root execution for both stage-0 and make after stage-0 self-escalates.
Intent: Align harness PASS/FAIL criteria with the root-only execution contract so identity checks confirm policy instead of legacy non-root expectations.
Constraints: Preserve phase markers and GITHUB_USER checks; keep marker strings deterministic and explicit (`user=root uid=0`).
Affects: `examples/decomk-selftest/fixtures/confrepo/Makefile`, `examples/decomk-selftest/devpod-local/run.sh`, `examples/decomk-selftest/codespaces/run.sh`, `TODO/007-devpod-gcp-selfhost-migration.md`.

Related design docs:
- `doc/isconf-design.md`
- `doc/decomk-design.md`
- `doc/image-management.md`
- `TODO/011-single-path-checkpoints.md`

## Goal

Define a decision-complete migration plan from GitHub Codespaces to DevPod on GCP so implementation can proceed without architecture ambiguity.

Success criteria:

- Teams can run the same devcontainer-based workflow in DevPod on GCP.
- decomk behavior is unchanged from the user perspective unless explicitly documented.
- We can cut over from Codespaces without blocking active development.

## Current state (Codespaces)

Today, decomk is bootstrapped using `postCreateCommand` with a reference script at `examples/devcontainer/postCreateCommand.sh`.

Current assumptions that we must preserve during migration:

- `decomk run` remains non-interactive.
- Default behavior is make execution as root when needed.
- State and log paths are controlled by `DECOMK_HOME` and `DECOMK_LOG_DIR`.

## Target architecture (DevPod + GCP)

For remote/self-hosted workspaces on GCP:

- Local DevPod CLI/Desktop is the control entrypoint.
- DevPod GCP provider creates or reuses a workspace VM in GCP.
- Docker (on that VM) runs the devcontainer workspace.
- DevPod starts `devpod-agent` in the workspace environment for command/session operations.

Intent clarification:

- We do not require a separate always-on management-plane container for this workflow.
- `devcontainer.json` remains the workspace contract.

## How DevPod works

This section captures the practical runtime model we are assuming for this migration.

### Local provider path (Docker)

When the Docker provider is selected:

- DevPod CLI/Desktop runs on the developer machine as the controller.
- `dockerd` is the runtime that actually builds and runs workspace containers.
- `devpod up` asks Docker to start the workspace defined by `devcontainer.json`.
- DevPod then starts `devpod-agent` in the workspace environment to handle command/session control operations.

### Remote provider path (GCP machine provider)

When the GCP machine provider is selected:

- DevPod on the developer machine controls workspace lifecycle.
- The provider creates or reuses a GCP VM.
- Docker on that VM runs the workspace container from `devcontainer.json`.
- DevPod connects through the provider path (including IAP-style tunneling where configured) and uses `devpod-agent` in the workspace environment for operations.

### What `devpod-agent` is

`devpod-agent` is the remote-side helper process used by DevPod for workspace control tasks. It is not a separate product that users install manually. In the Docker provider case, it runs in the local workspace environment; in the GCP provider case, it runs in the remote workspace environment.

### Management-plane clarification

- For normal local Docker-provider usage, no separate always-on DevPod management-plane container is required.
- If `devcontainer.json` uses Docker Compose, multiple service containers may exist, but those are workspace/application containers, not a separate DevPod management plane.

### Installation and runtime dependencies

- DevPod CLI or Desktop must be installed on the developer machine.
- A provider runtime is still required:
  - Local mode: Docker engine (`dockerd`).
  - Remote mode: GCP provider prerequisites plus Docker on the workspace VM.
- For GCP provider usage, developer authentication and project/zone/provider configuration must be set before `devpod up`.

Why this matters for decomk migration:

- Keep `devcontainer.json` as the portability contract across providers.
- Validate local DevPod Docker-provider behavior first, then add Codespaces parity checks.
- Defer remote GCP-provider automation until the migration decision is approved.
- Avoid hardcoding Codespaces-only assumptions in bootstrap and tests.

## Provider modes we support

We support two provider classes with one configuration contract:

- Local: Docker provider (`dockerd` on developer machine).
- Remote self-hosted: GCP machine provider (VM + Docker runtime).

The repo should avoid hardcoding assumptions that only hold in one provider class.

## GCP infrastructure design (full IaC)

Use Terraform as the source of truth for the GCP control plane and workspace foundation.

Proposed module layout:

- `infra/terraform/modules/network`
- `infra/terraform/modules/iam`
- `infra/terraform/modules/workspace_vm`
- `infra/terraform/modules/observability`
- `infra/terraform/environments/dev`
- `infra/terraform/environments/stage`
- `infra/terraform/environments/prod`

Required module outputs:

- `project_id`
- `region`
- `zone`
- `vpc_name`
- `subnet_name`
- `workspace_service_account_email`
- `iap_tunnel_enabled`
- `workspace_image`
- `workspace_labels`
- `log_sink_targets`

## IAM and security model

Required roles and boundaries:

- Developers: least-privilege ability to launch/attach their own workspaces.
- CI automation: scoped ability to validate Terraform and policy conformance.
- Workspace service account: runtime permissions needed for workspace lifecycle only.

Security requirements:

- Use IAP-based access patterns for remote connectivity.
- Do not rely on broad project-wide editor/admin roles for daily use.
- Tag and label all workspace resources for audit and budget tracking.

## Migration plan (dual-run then cutover)

Phase 1: Infrastructure and provider readiness

- Build Terraform modules and environment definitions.
- Validate provider profile setup and authentication path.

Phase 2: Pilot cohort

- Move a small set of active repos/users to DevPod-on-GCP.
- Record behavioral parity results and operational gaps.

Phase 3: Dual-run window

- Keep Codespaces and DevPod both available.
- Define default recommendation as “DevPod preferred, Codespaces fallback.”

Phase 4: Default switch

- Update docs and bootstrap references so DevPod-on-GCP is the default path.
- Keep Codespaces available for rollback only.

Phase 5: Codespaces decommission

- Remove Codespaces-specific operational dependencies after stability window closes.
- Archive final rollback runbook and disable unused hosted settings.

## Operational policies

Lifecycle policies:

- Enforce workspace inactivity timeouts.
- Enforce max workspace age for non-persistent environments.

Cost controls:

- Require labels for owner/repo/environment.
- Set quota guardrails and budget alerts.
- Implement idle cleanup for stale workspaces and disks.

Ownership model:

- Define on-call/owner for provider configuration.
- Define escalation path for IAM and quota failures.

## Validation matrix and acceptance tests

Required test scenarios:

- Local DevPod Docker provider: automated workspace bring-up and decomk bootstrap scenarios pass.
- Default make behavior: no password prompts during normal bootstrap.
- Path behavior: `DECOMK_HOME` and `DECOMK_LOG_DIR` produce expected state/log locations.
- Ownership invariants: stamps and generated state remain writable by intended dev user.
- Failure behavior: clear error when root-make is required but sudo capability is missing.
- Codespaces parity checks: run after local DevPod matrix is stable.
- GCP provider checks: deferred until a separate move-to-GCP decision is made.

Acceptance gate before cutover:

- Immediate gate: local DevPod Docker-provider matrix passes consistently.
- Follow-on gate: Codespaces parity checks pass before default recommendation changes.
- Remote GCP gates remain deferred pending separate approval.

## Rollback plan

Rollback trigger examples:

- Repeated provider auth failures.
- Quota/IAM incidents that block onboarding.
- Workspace reliability below agreed threshold.

Rollback steps:

- Restore Codespaces-first recommendation in docs and onboarding.
- Keep DevPod configs available but non-default.
- Preserve migration telemetry and incident notes for next attempt.

## Risks and mitigations

Risk: IAM drift or over-privilege.
Mitigation: Terraform-managed IAM with policy review gate.

Risk: Cost growth from orphaned workspaces.
Mitigation: TTL policies, budget alarms, and cleanup automation.

Risk: Quota exhaustion in target zones/projects.
Mitigation: capacity planning, quota monitoring, secondary-zone fallback.

Risk: Ownership mismatch in generated files.
Mitigation: run decomk with documented user/privilege model and explicit invariants.

Risk: Provider/version drift across teams.
Mitigation: pin provider and CLI versions in docs and CI checks.

Risk: Local self-hosted prebuild semantics diverge from Codespaces (`updateContentCommand` not run during local image build path).
Mitigation: Track deferred runtime-adaptation option (DevPod/reference-CLI wrapper or fork) and evaluate only after baseline self-hosting decision gates are met.
Implementation detail ownership: local freeze/parity mechanics, gates, and acceptance artifacts are tracked in TODO 011.

## Subtasks

- [ ] 007.1 Define Terraform root/module structure and environment variable contract.
- [ ] 007.2 Implement `network` module with VPC/subnet and outputs consumed by workspace modules.
- [ ] 007.3 Implement `iam` module with least-privilege roles for developers, CI, and workspace service accounts.
- [ ] 007.4 Implement `workspace_vm` module and document Docker runtime assumptions for DevPod.
- [ ] 007.5 Implement `observability` module for logs/metrics/budget labels and alerts.
- [ ] 007.6 Add `examples/devpod/` reference docs showing provider setup and `devpod up` flow.
- [ ] 007.7 Add migration runbook covering pilot, dual-run, cutover, and rollback.
- [x] 007.8 Define and automate local DevPod Docker-provider validation matrix in `examples/decomk-selftest/devpod-local` with decomk-native subtest orchestration.
- [ ] 007.9 Execute pilot cohort and capture parity findings vs Codespaces.
- [ ] 007.10 Perform default switch and keep rollback window open for one stabilization period.
- [ ] 007.11 Decommission Codespaces dependencies after stabilization and archive fallback documentation.
- [x] 007.12 Add automated Codespaces parity harness under `examples/decomk-selftest/codespaces`.
- [ ] 007.13 Revisit remote GCP-provider self-test scope after explicit move-to-GCP decision.
- [ ] 007.14 Evaluate deferred runtime-adaptation options for hook-inclusive prebuild snapshots (DevPod wrapper/fork vs reference-CLI wrapper) after baseline self-hosting cutover criteria are satisfied.
