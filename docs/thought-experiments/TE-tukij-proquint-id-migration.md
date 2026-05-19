# TE-tukij: proquint ID migration

TE ID: TE-tukij

## Decision Under Test

Should decomk replace numeric/timestamp coordination IDs with proquint handles
now, and if so should the migration be done as one mechanical pass using a
local mapping/xref toolchain?

## Assumptions

- decomk is still pre-production, so preserving backward-compatible numeric
  coordination IDs is less valuable than removing confusing allocation rules now.
- Coordination artifacts are human-edited documents and comments, not runtime
  protocols.
- Operational identifiers such as checkpoint block names, image tags, run IDs,
  stamp files, ports, UIDs, dates, and versions are outside this migration.
- Sibling repos already demonstrate the desired family format:
  `TODO-<handle>`, `TE-<handle>`, `DI-<handle>`, and `<handle>.N` subtasks.

## Alternatives

1. Keep numeric/timestamp IDs.
2. Migrate only new artifacts and leave existing files numeric.
3. Migrate coordination IDs in one pass with a mapping TSV and root xref.
4. Migrate all visible numbers, including operational names.

## Scenario Analysis

### Normal Operation

Keeping numeric IDs keeps the current tree stable but preserves a global
sequencing burden and makes cross-repo references harder to scan aloud.
Migrating only new artifacts creates a mixed system where authors must remember
which naming scheme applies to which document. A one-pass coordination-only
migration gives the repo a single active convention while preserving lookup data
for old references. Migrating operational names would blur coordination IDs with
runtime contracts and create unnecessary behavior risk.

### Failure and Incomplete Writes

A hand migration risks missed references and untraceable renames. A tool-driven
one-pass migration gives us a mapping TSV before renames, uses `git mv` for file
identity, and creates a root xref for human recovery. If the migration fails
mid-pass, the mapping and git diff expose exactly what changed. A gradual
migration spreads that risk across many future edits.

### Concurrent Actors and Mixed Versions

During a short migration branch, mixed numeric/proquint references can exist in
uncommitted state. After commit, active guidance and owner files should use only
proquint IDs. The xref is deliberately historical and is allowed to retain old
numeric IDs. Runtime code does not consume TODO/TE/DI IDs, so mixed versions do
not create protocol compatibility problems.

### Long-Horizon Evolution

Proquints remove the need to allocate the next integer and avoid timestamp
collision/ordering debates. The local `mint-handle` corpus scan gives future
authors a collision-checked path without a central registry. If the corpus grows
beyond proquint-1 comfort, the tools already allow proquint-2 handles.

### Trust Boundaries

The migration does not change runtime trust boundaries. The only new executable
surface is repo-local maintenance tooling, which is reviewable and tested. The
mapping/xref files are lookup aids, not runtime policy inputs.

### Scale Effects

The repo has enough references that manual edits are likely to miss citations.
One-pass tooling has a higher up-front cost, but it amortizes into stable future
maintenance and avoids a long-lived mixed convention. The generated xref and TSV
are small text files and do not create meaningful storage overhead.

## Conclusions

Use alternative 3: migrate coordination IDs in one pass with local tools,
mapping TSV authority, and root xref. Reject alternative 1 because it keeps the
allocation problem. Reject alternative 2 because mixed active conventions are
more confusing than a single migration. Reject alternative 4 because operational
identifiers have runtime meaning and must not be mechanically renamed.

## Implications for TODOs and DIs

- This TE supports `TODO-zifur`.
- The locked decision is `DI-puhon`.
- Future TODO, TE, and DI artifacts should use proquint handles.
- Historical numeric IDs remain discoverable through `numeric-proquint-xref.md`
  and `tools/migrate-handles/mapping.tsv`.
