# TODO-zifur: proquint ID migration

## Decision Intent Log

ID: DI-puhon
Date: 2026-05-18 18:38:20 -0700
Status: active
Decision: Replace numeric/timestamp coordination IDs with proquint handles using one mechanical migration pass and preserve legacy lookup through a root xref plus tool-owned mapping TSV.
Intent: Make TODO, TE, and DI identifiers short, pronounceable, non-sequential, and collision-checkable without coupling coordination artifacts to global numeric allocation or timestamps.
Constraints:
- Scope is coordination IDs only: TODO files, TODO subtask IDs, inline DI IDs, TE files, and their references.
- Operational names remain unchanged, including `BlockNN`, image tags, run IDs, stamp files, UIDs, dates, ports, and semantic versions.
- The migration uses the CSWG family format: `TODO/TODO-<handle>-<slug>.md`, `docs/thought-experiments/TE-<handle>-<slug>.md`, `DI-<handle>`, and `<handle>.N` subtask IDs.
- The migration is one-pass and includes the currently uncommitted TODO-danih DI entry in the migrated corpus.
- `tools/migrate-handles/mapping.tsv` is the machine-readable migration authority, while `numeric-proquint-xref.md` is the human lookup artifact.
Affects:
- `TODO/`
- `TODO/TODO.md`
- `docs/thought-experiments/`
- `numeric-proquint-xref.md`
- `tools/mint-handle/`
- `tools/migrate-handles/`
- `tools/sweep-citations/`
- repo-wide text references to legacy coordination IDs

## Context

This work switches decomk coordination artifacts from integer/timestamp IDs to
proquint-style handles consistent with the direction already used in sibling
repositories. The migration is intentionally mechanical: preserve old-to-new
lookup data, rename owner files, rewrite local owner fields, and sweep active
repo citations.

## Scope

- [x] zifur.1 Add local proquint maintenance tools under `tools/`.
- [x] zifur.2 Write the thought experiment for decomk-specific migration scope and risks.
- [x] zifur.3 Generate `tools/migrate-handles/mapping.tsv` from the pre-migration corpus.
- [x] zifur.4 Generate `numeric-proquint-xref.md` from the mapping for human lookup.
- [x] zifur.5 Rename legacy TODO and TE files with `git mv`.
- [x] zifur.6 Rewrite inline DI owner IDs and TODO subtask IDs to proquint form.
- [x] zifur.7 Sweep active repo references from legacy IDs to proquint IDs.
- [x] zifur.8 Update AGENTS/TODO guidance so new coordination artifacts use proquint IDs.
- [x] zifur.9 Validate the migration with tool tests, repo tests, citation sweeps, and comment audits.
