# Intent: Provide a deterministic codespaces selftest base image with a non-root
# dev user and passwordless sudo so decomk privilege behavior is exercised the
# same way as in the local DevPod harness.
# Source: DI-007-20260424-215415 (TODO/007)
FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    golang-go \
    make \
    sudo \
 && rm -rf /var/lib/apt/lists/*

# Intent: Some devcontainer base images already include uid 1000 (for example
# `vscode`), so selftest user provisioning must be idempotent while enforcing
# one deterministic identity contract: username `dev` with uid `1000`.
# Source: DI-007-20260424-215415 (TODO/007)
RUN set -eux; \
    existing_uid_user=""; \
    if uid_entry="$(getent passwd 1000 2>/dev/null)"; then \
      existing_uid_user="${uid_entry%%:*}"; \
    fi; \
    if id -u dev >/dev/null 2>&1; then \
      dev_uid="$(id -u dev)"; \
      if [ "$dev_uid" != "1000" ]; then \
        echo "expected dev uid 1000, got $dev_uid" >&2; \
        exit 1; \
      fi; \
      usermod --shell /bin/bash dev; \
    elif [ -n "$existing_uid_user" ]; then \
      if [ "$existing_uid_user" != "dev" ]; then \
        usermod --login dev "$existing_uid_user"; \
      fi; \
      if getent group dev >/dev/null 2>&1; then \
        usermod --gid dev dev; \
      elif getent group "$existing_uid_user" >/dev/null 2>&1; then \
        groupmod --new-name dev "$existing_uid_user"; \
      fi; \
      current_home="$(getent passwd dev | cut -d: -f6)"; \
      if [ "$current_home" != "/home/dev" ]; then \
        usermod --home /home/dev --move-home dev; \
      fi; \
      usermod --shell /bin/bash dev; \
    else \
      useradd --create-home --shell /bin/bash --uid 1000 dev; \
    fi; \
    echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev; \
    chmod 0440 /etc/sudoers.d/dev

RUN mkdir -p /workspaces \
 && chown -R dev:dev /workspaces
