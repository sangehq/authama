# Containerised dev environment for authama (Authentication Access Manager).
#
# Carried over from auth-token-manager's dev container and adapted for Go:
# commands run INSIDE the container against the live source, and the files the
# tool writes land in the container user's home, which compose.yaml binds to a
# host directory. Enter with `make docker-shell`.
#
# - golang:<go.mod version> (Debian), pinned by digest, plus the few tools the suite and the tool
#   exec: git, make, openssh-client (ssh-keygen/ssh, for the SSH side and the
#   e2e), ca-certificates. Nothing else.
# - The Go module and build caches live under /go and /home/dev/.cache,
#   OUTSIDE /workspace, so the bind-mounted source cannot shadow them.
# - Non-root user `dev` with a build-time UID/GID, so files written into the
#   mounted home keep the host user's ownership.
# - authama's config home is /home/dev/.config/authama and its state home
#   /home/dev/.local/state/authama, so both land in that mounted home.

# The base image. `base` is golang pinned by tag and digest, so every build
# starts from the same bytes, and Dependabot's docker updater moves the tag and
# digest together (.github/dependabot.yml). BASE_STAGE names the stage the
# image is built on; compose.yaml sets it only when AUTHAMA_GO_VERSION is set.
ARG BASE_STAGE=base
# GO_VERSION is read only by the deprecated `base-unpinned` stage below.
ARG GO_VERSION=1.27

FROM golang:1.27@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS base

# DEPRECATED, UNPINNED: golang:${GO_VERSION} by tag alone, with no digest, so
# what it pulls can change under the same tag and Dependabot cannot update it.
# It exists only so AUTHAMA_GO_VERSION keeps working (compose.yaml selects it
# when that variable is set). Removable no earlier than v1.0.0
# (.claude/design/authama/audits/deprecations.md). BuildKit builds only the
# stage that is used, so the default build never pulls this one.
FROM golang:${GO_VERSION} AS base-unpinned

FROM ${BASE_STAGE} AS dev

LABEL org.opencontainers.image.title="authama-dev" \
      org.opencontainers.image.description="Authentication Access Manager: SSH keys and config, and auth tokens - dev container" \
      org.opencontainers.image.source="https://github.com/sangehq/authama" \
      org.opencontainers.image.licenses="MIT"

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      git \
      make \
      openssh-client \
 && rm -rf /var/lib/apt/lists/*

ARG UID=1000
ARG GID=1000
RUN set -eux; \
    getent group "${GID}" >/dev/null || groupadd -g "${GID}" dev; \
    useradd -m -u "${UID}" -g "${GID}" -s /bin/bash dev; \
    mkdir -p /workspace; \
    chown -R "${UID}:${GID}" /workspace /go

USER dev
WORKDIR /workspace

# Warm the module cache from go.mod/go.sum alone, so a source edit does not
# re-download every dependency.
COPY --chown=${UID}:${GID} src/go.mod src/go.sum src/
RUN go -C src mod download

# Build authama into the image as well, so `authama` works before the
# first `make build`. Inside the container `make build` rebuilds from the live
# source into build/ in the bind mount.
COPY --chown=${UID}:${GID} src/ src/
RUN go -C src build -trimpath -o /go/bin/authama ./cmd/authama

HEALTHCHECK --interval=60s --timeout=5s --start-period=5s --retries=3 \
  CMD ["authama", "version"]

# Idle until someone execs in (compose sets init: true to reap and forward signals).
CMD ["sleep", "infinity"]
