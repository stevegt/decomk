# This Dockerfile is intended for genesis image bootstrap only.
# Keep it minimal: install only prerequisites needed for decomk stage-0
# (`bash`, `git`, `go`, `make`, `sudo`) and writable decomk state directories.
#
# After your genesis image is stable, stop using this Dockerfile and switch
# `.devcontainer/devcontainer.json` to an `image:` stanza that points at your
# stable channel tag (for example `ghcr.io/<org>/<repo>:stable`).

FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      git \
      golang-go \
      make \
      sudo \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/decomk /var/log/decomk \
 && chown -R vscode:vscode /var/decomk /var/log/decomk
