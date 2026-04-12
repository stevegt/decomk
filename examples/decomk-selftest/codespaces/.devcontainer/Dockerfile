# Intent: Provide a deterministic codespaces selftest base image with a non-root
# dev user and passwordless sudo so decomk privilege behavior is exercised the
# same way as in the local DevPod harness.
# Source: DI-007-20260412-230000 (TODO/007)
FROM golang:1.24-bookworm

RUN apt-get update \
 && apt-get install -y --no-install-recommends sudo ca-certificates \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash --uid 1000 dev \
 && echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev \
 && chmod 0440 /etc/sudoers.d/dev

RUN mkdir -p /workspaces \
 && chown -R dev:dev /workspaces
