# Intent: Keep the codespaces selftest image minimal while making the stage-0
# remote-identity contract explicit via DECOMK_REMOTE_* image defaults.
# Source: DI-007-20260425-110755 (TODO/007)
FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    golang-go \
    make \
    sudo \
 && rm -rf /var/lib/apt/lists/*

# Intent: Before changing these values, inspect the base image identities first.
# Example: docker run --rm -it mcr.microsoft.com/devcontainers/base:ubuntu-24.04 cat /etc/passwd
# Source: DI-007-20260425-110755 (TODO/007)
ENV DECOMK_REMOTE_USER=vscode
ENV DECOMK_REMOTE_UID=1000

RUN mkdir -p /workspaces \
 && chown -R "${DECOMK_REMOTE_UID}:${DECOMK_REMOTE_UID}" /workspaces
