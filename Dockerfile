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
    python3 \
    python3-venv \
    python3-pip \
    sudo \
    tzdata \
    vim-tiny \
 && rm -rf /var/lib/apt/lists/*

#
# Arch (will be "amd64" or "arm64"):
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
# Claude Code:
#
ARG CLAUDE_CODE_VERSION=2.0.64
RUN CLAUDE_URL="https://storage.googleapis.com/claude-code-dist-86c565f3-f756-42ad-8dfa-d59b1c096819/claude-code-releases/${CLAUDE_CODE_VERSION}/linux-${TARGETARCH}/claude" \
 && curl -fsSL "${CLAUDE_URL}" -o /usr/local/bin/claude \
 && chmod +x /usr/local/bin/claude \
 && claude --version
ENV DISABLE_AUTOUPDATER=1

#
# Cursor Agent:
#
ARG CURSOR_AGENT_VERSION=2025.11.25-d5b3271
RUN set -eux \
 && case "${TARGETARCH}" in \
      amd64) CURSOR_ARCH="x64" ;; \
      arm64) CURSOR_ARCH="arm64" ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
 && CURSOR_VERSIONS_DIR="/usr/local/share/cursor-agent/versions" \
 && mkdir -p "${CURSOR_VERSIONS_DIR}" \
 && CURSOR_TEMP_DIR="$(mktemp -d "${CURSOR_VERSIONS_DIR}/.tmp-${CURSOR_AGENT_VERSION}-XXXXXX")" \
 && CURSOR_URL="https://downloads.cursor.com/lab/${CURSOR_AGENT_VERSION}/linux/${CURSOR_ARCH}/agent-cli-package.tar.gz" \
 && curl -fSL "${CURSOR_URL}" \
    | tar --strip-components=1 -xzf - -C "${CURSOR_TEMP_DIR}" \
 && CURSOR_FINAL_DIR="${CURSOR_VERSIONS_DIR}/${CURSOR_AGENT_VERSION}" \
 && rm -rf "${CURSOR_FINAL_DIR}" \
 && mv "${CURSOR_TEMP_DIR}" "${CURSOR_FINAL_DIR}" \
 && ln -sf "${CURSOR_FINAL_DIR}/cursor-agent" /usr/local/bin/cursor-agent \
 && chmod -R a+rX "${CURSOR_FINAL_DIR}" \
 && chmod +x "${CURSOR_FINAL_DIR}/cursor-agent" \
 && cursor-agent --help >/dev/null

#
# Aider:
#
ARG AIDER_VERSION=0.86.1
ARG PIPX_VERSION=1.7.1
ENV PIPX_BIN_DIR=/usr/local/bin
ENV PIPX_HOME=/opt/pipx
ENV AIDER_CHECK_UPDATE=false
RUN set -eux \
 && mkdir -p "${PIPX_HOME}" \
 && python3 -m pip install --no-cache-dir --break-system-packages "pipx==${PIPX_VERSION}" \
 && pipx install "aider-chat==${AIDER_VERSION}" \
 && aider --version

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
