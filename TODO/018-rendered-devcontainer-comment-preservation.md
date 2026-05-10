# TODO 018 - rendered devcontainer comment preservation

## Decision Intent Log

ID: DI-018-20260507-153000
Date: 2026-05-07 15:30:00 -0700
Status: active
Decision: Add registry-owned generated-field comments to `decomk branch render` using a `devcontainer.comments` registry field keyed by rendered top-level devcontainer field name.
Intent: Preserve downstream DI provenance and behavior-level comments when `.devcontainer/devcontainer.json` becomes a generated artifact, without hardcoding downstream repo comments into decomk itself.
Constraints: Keep comment data in the channel registry; emit comments immediately before matching rendered fields; fail fast if a configured comment targets a field that was not rendered; cover `comments.overrideCommand` with focused tests; keep checkpoint promotion behavior unchanged.
Affects: `cmd/decomk/branch.go`, `cmd/decomk/branch_test.go`, `TODO/018-rendered-devcontainer-comment-preservation.md`, `TODO/TODO.md`

ID: DI-018-20260507-204852
Date: 2026-05-07 20:48:52 -0700
Status: active
Decision: Implement `devcontainer.comments` with `github.com/tailscale/hujson` AST mutation, preserve the current rendered layout, validate comment keys against the selected channel's actual rendered top-level fields, and update module metadata only via `go mod tidy`.
Intent: Lock the concrete renderer strategy, naming family, and dependency update path so TODO 018 can be implemented without inventing new code paths or layout policy mid-change.
Constraints: Use helper names in the `write/keys` family (`planBranchFields`, `writeBranchDevcontainer`, `validateCommentKeys`, `plannedFields`, `fieldKeys`); edit only `cmd/decomk/branch.go`, `cmd/decomk/branch_test.go`, and this TODO file plus `go.mod`/`go.sum` via `go mod tidy`; use `/tmp/decomk-gocache` and `/tmp/decomk-gomodcache` for Go tool caches; keep checkpoint behavior unchanged.
Affects: `cmd/decomk/branch.go`, `cmd/decomk/branch_test.go`, `TODO/018-rendered-devcontainer-comment-preservation.md`, `go.mod`, `go.sum`
Supersedes: DI-018-20260507-153000

ID: DI-018-20260507-210842
Date: 2026-05-07 21:08:42 -0700
Status: superseded
Decision: Keep the hujson-required Go minimum at the first patch-qualified Go 1.23 release by setting the module directive to `go 1.23.0`.
Intent: Prevent older bootstrap Go commands from resolving the non-existent `go1.23` toolchain while preserving the narrow language-version bump required by hujson.
Constraints: Do not change stage-0 install semantics or selftest image policy for this fix; keep the change limited to module metadata and this TODO evidence.
Affects: `go.mod`, `TODO/018-rendered-devcontainer-comment-preservation.md`

## Goal

Teach `decomk branch render` to preserve intentional generated devcontainer comments from `.decomk/channels.json`, so downstream repos can keep comment and DI provenance while treating `.devcontainer/devcontainer.json` as a rendered artifact.

## Requested implementation

- Add `devcontainer.comments` to the branch registry schema.
- Use type `map[string][]string`, where each key is a rendered top-level devcontainer field name such as `overrideCommand`.
- Insert each configured comment block immediately before the matching rendered field.
- Prefix each configured line with JSONC `// ` in generated output.
- Fail if a configured comment key names a field that is not present in the rendered devcontainer file.
- Keep `decomk checkpoint tag` and checkpoint promotion behavior unchanged.

## Acceptance tests

- Add focused tests proving `comments.overrideCommand` renders immediately before `"overrideCommand"`.
- Add or update tests proving stale-render checks include comment changes.
- Keep existing tests proving `testing` and `stable` reject `DECOMK_TOOL_URI` values ending in `@latest`.

## Downstream consumer

`/home/stevegt/lab/decomk-conf-cswg` needs this so its channel registry can preserve the existing `DI-004-20260430-182956` rationale near the rendered `overrideCommand` field.
