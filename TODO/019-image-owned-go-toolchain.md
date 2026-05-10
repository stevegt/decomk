# TODO 019 - image-owned Go toolchain

## Decision Intent Log

ID: DI-019-20260510-111726
Date: 2026-05-10 11:17:26 -0700
Status: active
Decision: Make decomk-owned bootstrap images responsible for selecting and constraining the Go toolchain, including image-level `GOTOOLCHAIN=local`, while stage0 only consumes the image-provided `go`.
Intent: Prevent implicit Go compiler downloads during stage0 bootstrap without baking image-specific package policy into the generic stage0 script.
Constraints: Keep the available `mcr.microsoft.com/devcontainers/base:ubuntu-24.04` base for now; install explicit Ubuntu `golang-1.23-go` instead of default `golang-go` where decomk-owned images must build current decomk; set image-level `PATH=/usr/lib/go-1.23/bin:$PATH`; set image-level `GOTOOLCHAIN=local`; do not add `GOTOOLCHAIN=local` to stage0.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, `cmd/decomk/templates/confrepo.Dockerfile.tmpl`, generated confrepo/examples as needed, stage0/selftest validation, `TODO/018-rendered-devcontainer-comment-preservation.md`, `go.mod`.

ID: DI-019-20260510-112226
Date: 2026-05-10 11:22:26 -0700
Status: active
Decision: Supersede the narrow TODO 018 toolchain-name mitigation with image-owned Go 1.23 package selection and image-level `GOTOOLCHAIN=local`.
Intent: Keep `go.mod` at the hujson-required Go 1.23 floor while making the container image, not stage0 or the Go command's auto-download path, responsible for providing a compatible compiler.
Constraints: Leave stage0 free of `GOTOOLCHAIN=local`; update decomk-owned Dockerfiles/templates plus generated examples; keep post-commit Codespaces selftest evidence as a separate verification step.
Affects: `.devcontainer/codespaces-selftest/Dockerfile`, `cmd/decomk/templates/confrepo.Dockerfile.tmpl`, `examples/confrepo/.devcontainer/Dockerfile`, `cmd/decomk/toolchain_dockerfile_test.go`, `TODO/019-image-owned-go-toolchain.md`, `TODO/018-rendered-devcontainer-comment-preservation.md`, `go.mod`.
Supersedes: DI-018-20260507-210842

ID: DI-019-20260510-113632
Date: 2026-05-10 11:36:32 -0700
Status: active
Decision: Use the module directive `go 1.23`, not `go 1.23.0`.
Intent: Keep the module directive aligned with Go module convention and the hujson-required Go 1.23 floor while leaving exact compiler selection to image-owned package policy.
Constraints: Do not reintroduce Go 1.22 compatibility work; do not encode patch-level compiler selection in `go.mod`; keep image-level `golang-1.23-go` and `GOTOOLCHAIN=local` as the runtime bootstrap controls.
Affects: `go.mod`, `TODO/019-image-owned-go-toolchain.md`.

## Goal

Make decomk bootstrap images provide a deterministic Go toolchain that is new
enough for the decomk module, while keeping stage0 generic and preventing Go's
automatic toolchain download path during container startup.

## Current findings

- `mcr.microsoft.com/devcontainers/base:ubuntu-26.04` is not currently available; `docker run --rm -it mcr.microsoft.com/devcontainers/base:ubuntu-26.04 bash` reports `manifest tagged by "ubuntu-26.04" is not found`.
- Ubuntu 24.04 remains the current devcontainers base for decomk-owned selftest images.
- Ubuntu 24.04 has explicit `golang-1.23-go` packages in noble security/updates (`1.23.1-1~24.04.1`) even though default `golang-go` resolves to Go 1.22.
- Current `github.com/tailscale/hujson` usage raises decomk's module minimum to Go 1.23.
- Image policy should own `GOTOOLCHAIN=local`; stage0 should not carry image-specific compiler-download policy.
- Local Docker validation built the Codespaces selftest image and verified `go` resolves to `/usr/lib/go-1.23/bin/go`, `go version` reports `go1.23.1 linux/amd64`, and `go env GOTOOLCHAIN` reports `local`.

## Scope

In scope:

- Updating decomk-owned Dockerfiles/templates that build decomk from source.
- Making Go 1.23 selection explicit in image configuration.
- Proving stage0 uses image-provided Go without implicit toolchain downloads.
- Resolving the uncommitted TODO 018/go.mod diagnostic change consistently with this TODO.

Out of scope:

- Switching to Ubuntu 26.04 before a devcontainers base tag exists.
- Replacing `github.com/tailscale/hujson` solely to preserve Go 1.22 compatibility.
- Adding `GOTOOLCHAIN=local` to stage0.

## Subtasks

- [x] 019.1 Record reproducible evidence that `mcr.microsoft.com/devcontainers/base:ubuntu-26.04` is unavailable.
- [x] 019.2 Record exact Ubuntu 24.04 package candidate evidence for `golang-1.23-go`.
- [x] 019.3 Update the Codespaces selftest Dockerfile to install `golang-1.23-go` and export image-level `PATH` plus `GOTOOLCHAIN=local`.
- [x] 019.4 Update producer Dockerfile template policy for images that must build decomk from source.
- [x] 019.5 Add or adjust tests proving image-owned Go policy; post-push log evidence for no `go: downloading go1...` remains 019.7.
- [x] 019.6 Decide whether `DI-018-20260507-210842` should be superseded by this TODO or revised before commit.
- [ ] 019.7 Run Codespaces selftest after commit/push and verify there is no implicit Go toolchain download in preserved logs.

## Acceptance criteria

- Decomk-owned bootstrap images use explicit Go 1.23 tooling on Ubuntu 24.04.
- `GOTOOLCHAIN=local` is set by Dockerfile/image configuration, not by stage0.
- Stage0 remains generic and production-identical across image policies.
- Selftest evidence shows no implicit Go toolchain download during bootstrap.
- TODO 018 and `go.mod` are left in a coherent state with this image-owned toolchain decision.
