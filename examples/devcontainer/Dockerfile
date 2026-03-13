# Intent: Provide a standalone base image for the generated production example
# devcontainer so users can run stage-0 bootstrap without first supplying their
# own Dockerfile.
# Source: DI-001-20260313-183500 (TODO/001)
FROM golang:1.24-bookworm

RUN apt-get update \
 && apt-get install -y --no-install-recommends sudo ca-certificates git \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash --uid 1000 dev \
 && echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev \
 && chmod 0440 /etc/sudoers.d/dev

RUN mkdir -p /workspaces /var/decomk /var/log/decomk \
 && chown -R dev:dev /workspaces /var/decomk /var/log/decomk
