# TODO 007 - decomk: migrate from Codespaces to DevPod on self-hosted GCP

## Decision Intent Log

ID: DI-007-20260309-114541
Date: 2026-03-09 11:45:41
Status: active
Decision: Standardize devcontainer execution on DevPod with a GCP machine provider, and migrate from Codespaces using a dual-run transition.
Intent: Keep the devcontainer contract intact while moving control, cost, and operational policy to self-hosted GCP infrastructure.
Constraints: Preserve decomk non-interactive behavior (no sudo prompts), preserve current make-as-root default behavior, and preserve `DECOMK_HOME` and `DECOMK_LOG_DIR` semantics.
Affects: `TODO/007-devpod-gcp-selfhost-migration.md`, `TODO/TODO.md`, future `examples/devpod/*`, future `infra/terraform/*`, `README.md`.

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
- Validate parity in both local Docker-provider and remote GCP-provider paths.
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

- Local Docker provider: workspace boots and decomk bootstrap completes.
- GCP provider: workspace boots and decomk bootstrap completes.
- Default make behavior: no password prompts during normal bootstrap.
- Path behavior: `DECOMK_HOME` and `DECOMK_LOG_DIR` produce expected state/log locations.
- Ownership invariants: stamps and generated state remain writable by intended dev user.
- Failure behavior: clear error when root-make is required but sudo capability is missing.
- Parity checks: selected repo produces equivalent bootstrap outputs in Codespaces and DevPod.

Acceptance gate before cutover:

- All required scenarios pass in pilot and dual-run windows.
- Documented rollback path is tested once successfully.

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

## Subtasks

- [ ] 007.1 Define Terraform root/module structure and environment variable contract.
- [ ] 007.2 Implement `network` module with VPC/subnet and outputs consumed by workspace modules.
- [ ] 007.3 Implement `iam` module with least-privilege roles for developers, CI, and workspace service accounts.
- [ ] 007.4 Implement `workspace_vm` module and document Docker runtime assumptions for DevPod.
- [ ] 007.5 Implement `observability` module for logs/metrics/budget labels and alerts.
- [ ] 007.6 Add `examples/devpod/` reference docs showing provider setup and `devpod up` flow.
- [ ] 007.7 Add migration runbook covering pilot, dual-run, cutover, and rollback.
- [ ] 007.8 Define and automate the validation matrix for local Docker and remote GCP providers.
- [ ] 007.9 Execute pilot cohort and capture parity findings vs Codespaces.
- [ ] 007.10 Perform default switch and keep rollback window open for one stabilization period.
- [ ] 007.11 Decommission Codespaces dependencies after stabilization and archive fallback documentation.
