# This Dockerfile is intended for genesis image bootstrap only.
# Keep it minimal: install only prerequisites needed for decomk stage-0
# (`bash`, `git`, `go`, `make`, `sudo`) plus one canonical non-root identity
# (`dev`, uid 1000) and writable decomk state directories.
#
# After your genesis image is stable, stop using this Dockerfile and switch
# `.devcontainer/devcontainer.json` to an `image:` stanza that points at your
# stable channel tag (for example `ghcr.io/<org>/<repo>:stable`).

FROM golang:1.24-bookworm

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      git \
      make \
      sudo \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash --uid 1000 dev \
 && echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev \
 && chmod 0440 /etc/sudoers.d/dev

RUN mkdir -p /workspaces /var/decomk /var/log/decomk \
 && chown -R dev:dev /workspaces /var/decomk /var/log/decomk
