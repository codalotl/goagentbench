#
# Base Image
#
FROM ubuntu:24.04

#
# System Dependencies
#
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
    bash-completion \
    build-essential \
    ca-certificates \
    curl \
    file \
    git \
    less \
    openssh-client \
    procps \
    sudo \
    tzdata \
    vim-tiny \
 && rm -rf /var/lib/apt/lists/*

#
# Arch:
#
ARG TARGETARCH

#
# Go
#
ARG GO_VERSION=1.25.5
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" -o /tmp/go.tgz \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && rm /tmp/go.tgz
ENV GOPATH=/home/runner/go
ENV PATH=${GOPATH}/bin:/usr/local/go/bin:${PATH}

#
# Codex CLI:
#
ARG CODEX_VERSION=0.66.0
RUN case "${TARGETARCH}" in \
      amd64) CODEX_ARCH="x86_64-unknown-linux-gnu" ;; \
      arm64) CODEX_ARCH="aarch64-unknown-linux-gnu" ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
 && CODEX_TGZ="codex-${CODEX_ARCH}.tar.gz" \
 && CODEX_URL="https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION}/${CODEX_TGZ}" \
 && curl -fsSL "${CODEX_URL}" -o "/tmp/${CODEX_TGZ}" \
 && tar -xzf "/tmp/${CODEX_TGZ}" -C /usr/local/bin \
 && mv "/usr/local/bin/codex-${CODEX_ARCH}" /usr/local/bin/codex \
 && rm "/tmp/${CODEX_TGZ}" \
 && codex --version

#
# User / Working Directory
#
RUN useradd -m -s /bin/bash runner \
 && mkdir -p /workspace \
 && chown -R runner:runner /workspace
USER runner
WORKDIR /workspace

#
# Agent-specific user setup:
#

# So that we can mount a file (auth.json) inside .codex
RUN mkdir -p /home/runner/.codex

#
# Entrypoint
#
CMD ["/bin/bash"]
