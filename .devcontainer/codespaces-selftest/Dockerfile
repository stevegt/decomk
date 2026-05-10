# Intent: Keep the codespaces selftest image minimal while making the stage-0
# remote-identity contract explicit via DECOMK_REMOTE_* image defaults.
# Source: DI-007-20260425-110755 (TODO/007)
FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    golang-1.23-go \
    make \
    sudo \
 && rm -rf /var/lib/apt/lists/*

# Intent: Keep Go toolchain ownership in the image so stage0 consumes a
# deterministic compiler and never asks the Go command to auto-download one.
# Source: DI-019-20260510-111726 (TODO/019)
ENV PATH=/usr/lib/go-1.23/bin:$PATH
ENV GOTOOLCHAIN=local
RUN ln -sf /usr/lib/go-1.23/bin/go /usr/local/bin/go \
 && ln -sf /usr/lib/go-1.23/bin/gofmt /usr/local/bin/gofmt

# Intent: Before changing these values, inspect the base image identities first.
# Example: docker run --rm -it mcr.microsoft.com/devcontainers/base:ubuntu-24.04 cat /etc/passwd
# Source: DI-007-20260425-110755 (TODO/007)
ENV DECOMK_REMOTE_USER=vscode
ENV DECOMK_REMOTE_UID=1000

RUN mkdir -p /workspaces \
 && chown -R "${DECOMK_REMOTE_UID}:${DECOMK_REMOTE_UID}" /workspaces
