# Intent: Provide a deterministic minimal base image for phase lifecycle
# evaluation with a non-root dev user and passwordless sudo so hook behavior is
# measured consistently across local DevPod and Codespaces runs.
# Source: DI-009-20260413-232813 (TODO/009)
FROM golang:1.24-bookworm

RUN apt-get update \
 && apt-get install -y --no-install-recommends sudo ca-certificates \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash --uid 1000 dev \
 && echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev \
 && chmod 0440 /etc/sudoers.d/dev

RUN mkdir -p /workspaces \
 && chown -R dev:dev /workspaces
