# TODO-vimoj: decomk-side Go toolchain boundary

## Decision Intent Log

ID: DI-fisof
Date: 2026-05-10 11:17:26 -0700
Status: active
Decision: Make decomk-owned bootstrap images responsible for selecting and constraining the Go toolchain, including image-level `GOTOOLCHAIN=local`, while stage0 only consumes the image-provided `go`.
Intent: Prevent implicit Go compiler downloads during stage0 bootstrap without baking image-specific package policy into the generic stage0 script.
Constraints: Keep the available `mcr.microsoft.com/devcontainers/base:ubuntu-24.04` base for now; install explicit Ubuntu `golang-1.23-go` instead of default `golang-go` where decomk-owned images must build current decomk; set image-level `PATH=/usr/lib/go-1.23/bin:$PATH`; set image-level `GOTOOLCHAIN=local`; do not add `GOTOOLCHAIN=local` to stage0.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, `cmd/decomk/templates/confrepo.Dockerfile.tmpl`, generated confrepo/examples as needed, stage0/selftest validation, `TODO/TODO-dajos-rendered-devcontainer-comment-preservation.md`, `go.mod`.

ID: DI-divaf
Date: 2026-05-10 11:22:26 -0700
Status: active
Decision: Supersede the narrow TODO-dajos toolchain-name mitigation with image-owned Go 1.23 package selection and image-level `GOTOOLCHAIN=local`.
Intent: Keep `go.mod` at the hujson-required Go 1.23 floor while making the container image, not stage0 or the Go command's auto-download path, responsible for providing a compatible compiler.
Constraints: Leave stage0 free of `GOTOOLCHAIN=local`; update decomk-owned Dockerfiles/templates plus generated examples; keep post-commit Codespaces selftest evidence as a separate verification step.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, `cmd/decomk/templates/confrepo.Dockerfile.tmpl`, `examples/confrepo/.devcontainer/Dockerfile`, `cmd/decomk/toolchain_dockerfile_test.go`, `TODO/TODO-vimoj-image-owned-go-toolchain.md`, `TODO/TODO-dajos-rendered-devcontainer-comment-preservation.md`, `go.mod`.
Supersedes: DI-lutuj

ID: DI-fudok
Date: 2026-05-10 11:36:32 -0700
Status: active
Decision: Use the module directive `go 1.23`, not `go 1.23.0`.
Intent: Keep the module directive aligned with Go module convention and the hujson-required Go 1.23 floor while leaving exact compiler selection to image-owned package policy.
Constraints: Do not reintroduce Go 1.22 compatibility work; do not encode patch-level compiler selection in `go.mod`; keep image-level `golang-1.23-go` and `GOTOOLCHAIN=local` as the runtime bootstrap controls.
Affects: `go.mod`, `TODO/TODO-vimoj-image-owned-go-toolchain.md`.

ID: DI-hibav
Date: 2026-05-10 11:41:03 -0700
Status: active
Decision: Expose the image-owned Go 1.23 toolchain through `/usr/local/bin/go` and `/usr/local/bin/gofmt` symlinks in decomk-owned Dockerfiles.
Intent: Keep stage0 generic while making root-escalated `command -v go` work under sudo `secure_path`, which can drop Dockerfile `ENV PATH=/usr/lib/go-1.23/bin:$PATH`.
Constraints: Do not add `GOTOOLCHAIN=local` or package-specific policy to stage0; keep Go version selection in image-owned Dockerfiles/templates; preserve explicit `golang-1.23-go` package selection.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, `cmd/decomk/templates/confrepo.Dockerfile.tmpl`, `examples/confrepo/.devcontainer/Dockerfile`, `cmd/decomk/toolchain_dockerfile_test.go`, `TODO/TODO-vimoj-image-owned-go-toolchain.md`.

ID: DI-mogov
Date: 2026-05-13 22:10:53 -0700
Status: active
Decision: Close TODO-vimoj as decomk-side provenance only; move ongoing production image toolchain policy ownership to the `decomk-conf-cswg` repo.
Intent: Keep decomk responsible for the generic stage0/toolchain boundary and generated-template guardrails, while keeping base-image, APT package, snapshot, symlink, and producer-image release policy in the conf repo that owns those images.
Constraints: Preserve existing DI links used by decomk comments and tests; do not continue production image planning in this repo; treat `/tmp/decomk-codespaces.eSnnoS` as the post-push Codespaces evidence for the decomk-side fix.
Affects: `TODO/TODO-vimoj-image-owned-go-toolchain.md`, `TODO/TODO.md`.
Supersedes: DI-fisof, DI-divaf, DI-hibav

## Goal

Keep decomk's generic stage0 contract clear: stage0 consumes an image-provided
`go` command and must not own image-specific compiler download or package
selection policy. The production image policy belongs in `decomk-conf-cswg`,
which owns the producer Dockerfile, APT snapshot strategy, and release tags.

## Current findings

- `mcr.microsoft.com/devcontainers/base:ubuntu-26.04` is not currently available; `docker run --rm -it mcr.microsoft.com/devcontainers/base:ubuntu-26.04 bash` reports `manifest tagged by "ubuntu-26.04" is not found`.
- Ubuntu 24.04 remains the current devcontainers base for decomk-owned selftest images.
- Ubuntu 24.04 has explicit `golang-1.23-go` packages in noble security/updates (`1.23.1-1~24.04.1`) even though default `golang-go` resolves to Go 1.22.
- Current `github.com/tailscale/hujson` usage raises decomk's module minimum to Go 1.23.
- Image policy should own `GOTOOLCHAIN=local`; stage0 should not carry image-specific compiler-download policy.
- Local Docker validation built the Codespaces selftest image and verified `go` resolves to `/usr/lib/go-1.23/bin/go`, `go version` reports `go1.23.1 linux/amd64`, and `go env GOTOOLCHAIN` reports `local`.
- Codespaces artifact `/tmp/decomk-codespaces.XrFR3m` shows `golang-1.23-go` installed successfully, no implicit `go: downloading go1...` line, and no decomk log directory; this points at root-stage0 Go lookup before log setup, consistent with sudo `secure_path` dropping Dockerfile PATH.
- Codespaces artifact `/tmp/decomk-codespaces.eSnnoS` shows `run_exit_code 0`, successful updateContent execution as root, successful diagnostic capture, and no implicit `go: downloading go1...` line.

## Ownership split

This TODO was initially too broad for the decomk repo. Keep only these
decomk-side responsibilities here:

- stage0 remains generic and never sets `GOTOOLCHAIN=local`;
- generated templates make the expected image/toolchain boundary explicit;
- selftests catch regressions that would reintroduce implicit Go toolchain
  downloads during bootstrap;
- DI links used by decomk comments and tests remain resolvable.

Move or keep ongoing production-image decisions in `decomk-conf-cswg`,
primarily under that repo's TODO-fuviv (`apt-pin` / package snapshots) and TODO-bitum (checkpoint-backed release image automation), including:

- base image selection;
- exact APT package names and versions;
- APT snapshot or OCI package-cache strategy;
- `/usr/local/bin/go` and `/usr/local/bin/gofmt` symlink policy for producer
  images;
- checkpoint image release and channel promotion evidence.

## Scope

In scope:

- Preserving decomk-side DI provenance for the already-implemented stage0 and
  template boundary.
- Keeping generated examples and tests aligned with the generic stage0
  contract.
- Recording the post-push Codespaces evidence that closed the decomk-side
  regression.

Out of scope:

- Switching to Ubuntu 26.04 before a devcontainers base tag exists.
- Replacing `github.com/tailscale/hujson` solely to preserve Go 1.22 compatibility.
- Adding `GOTOOLCHAIN=local` to stage0.
- Continuing production producer-image planning in this repo.

## Subtasks

- [x] vimoj.1 Record reproducible evidence that `mcr.microsoft.com/devcontainers/base:ubuntu-26.04` is unavailable.
- [x] vimoj.2 Record exact Ubuntu 24.04 package candidate evidence for `golang-1.23-go`.
- [x] vimoj.3 Update the Codespaces selftest Dockerfile to install `golang-1.23-go` and export image-level `PATH` plus `GOTOOLCHAIN=local`.
- [x] vimoj.4 Update producer Dockerfile template policy for images that must build decomk from source.
- [x] vimoj.5 Add or adjust tests proving image-owned Go policy; post-push log evidence for no `go: downloading go1...` remains vimoj.7.
- [x] vimoj.6 Decide whether `DI-lutuj` should be superseded by this TODO or revised before commit.
- [x] vimoj.7 Run Codespaces selftest after commit/push and verify there is no implicit Go toolchain download in preserved logs.
- [x] vimoj.8 Re-scope this TODO as decomk-side provenance and hand off ongoing production image policy to `decomk-conf-cswg`.

## Acceptance criteria

- Stage0 remains generic and production-identical across image policies.
- Stage0 does not set `GOTOOLCHAIN=local`.
- Selftest evidence shows no implicit Go toolchain download during bootstrap.
- Production image package/base/snapshot policy is tracked in `decomk-conf-cswg`, not here.
- TODO-dajos and `go.mod` are left in a coherent state with this decomk-side boundary decision.
