# TODO 014 - apt package retention strategy

## Decision Intent Log

ID: DI-014-20260423-201731
Date: 2026-04-23 20:17:31
Status: active
Decision: Treat apt package version pinning alone as insufficient for deterministic rebuilds because pinned versions can age out of upstream apt servers; run a bakeoff between a managed mirror/snapshot path and resurrecting `~/lab/apt-tar/`.
Intent: Keep decomk-managed image and workspace rebuilds reproducible over time even when upstream package repositories churn or garbage-collect old versions.
Constraints: This TODO is planning-only in the current pass (no production rollout yet), must produce a decision-complete selection gate, and must feed TODO 011 deterministic pinning work.
Affects: `TODO/014-apt-package-retention-strategy.md`, `TODO/011-single-path-checkpoints.md`, `TODO/TODO.md`, future image-management implementation tasks.

## Goal

Define and choose a durable apt retention mechanism so decomk image
builds remain reproducible when upstream package versions expire.

## Problem statement

Pinned apt versions are not sufficient by themselves. A pinned package
can become unavailable from upstream mirrors, causing rebuild failures
even when Dockerfiles/Makefiles are unchanged.

## Scope

In scope:

- Capture and classify package-expiry failure modes.
- Evaluate two retention strategies with explicit decision criteria.
- Produce a decision gate that can be implemented without additional
  architectural choices.

Out of scope (this TODO pass):

- Shipping/operating production mirror infrastructure.
- Implementing `apt-tar` integration into decomk runtime scripts.

## Options under evaluation

1. Managed apt mirror/snapshot path
   - Keep required package sets available through retained snapshots.
2. Resurrect `~/lab/apt-tar/`
   - Archive and replay required apt artifacts independent of upstream
     retention windows.

## Evaluation criteria

- Rebuild reproducibility window (how long old builds remain rebuildable).
- Operational burden (maintenance, updates, on-call surface).
- Storage and bandwidth cost profile.
- CI/devcontainer ergonomics.
- Auditability and disaster-recovery characteristics.

## Subtasks

- [ ] 014.1 Capture at least two concrete failures where pinned apt versions were no longer available upstream.
- [ ] 014.2 Define acceptance criteria for “rebuildable” (time window, artifact guarantees, and verification method).
- [ ] 014.3 Prototype and document a managed mirror/snapshot workflow against one decomk checkpoint build.
- [ ] 014.4 Prototype and document `~/lab/apt-tar/` resurrection against the same build path.
- [ ] 014.5 Compare both options against the criteria and record final DI decision with rationale and constraints.
- [ ] 014.6 Write implementation handoff for the chosen path, including required repo changes and rollout sequence.

## Acceptance criteria

- TODO explicitly records that apt pinning alone is insufficient.
- Both retention options are documented and compared using the same criteria.
- A final decision gate exists (`014.5`) before implementation work begins.
- TODO 011 deterministic pinning work references this TODO as dependency.
