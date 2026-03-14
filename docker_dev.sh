#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="goagentbench"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOST_CODEX_AUTH="${HOME}/.codex/auth.json"
LINK_OPENAI_AUTH=1

for arg in "$@"; do
  case "${arg}" in
    nolinkopenai)
      LINK_OPENAI_AUTH=0
      ;;
  esac
done

# Build the image (cached) unless explicitly skipped.
if [[ -z "${GOAGENTBENCH_SKIP_BUILD:-}" ]]; then
  docker build -t "${IMAGE_NAME}" "${SCRIPT_DIR}"
fi

# Pass selected environment variables through to the container. Extend VARS_TO_PASS to add more.
VARS_TO_PASS=(CURSOR_API_KEY OPENAI_API_KEY ANTHROPIC_API_KEY XAI_API_KEY GEMINI_API_KEY)
ENV_ARGS=()
for var in "${VARS_TO_PASS[@]}"; do
  if [[ -n "${!var:-}" ]]; then
    ENV_ARGS+=("-e" "${var}=${!var}")
  fi
done
ENV_ARGS+=("-e" "GOAGENTBENCH_RESULTS=/host/results")

# Build mount args conditionally to avoid polluting test runs with user settings.
MOUNT_ARGS=("-v" "${SCRIPT_DIR}:/host")
if [[ "${LINK_OPENAI_AUTH}" -eq 1 && -n "${GOAGENTBENCH_CODEX_MOUNT_AUTH:-}" && -f "${HOST_CODEX_AUTH}" ]]; then
  MOUNT_ARGS+=("-v" "${HOST_CODEX_AUTH}:/home/runner/.codex/auth.json:rw")
fi

# NOTE: claude code works with claude -p if the ANTHROPIC_API_KEY is set, but tries to do onboarding if running in TUI mode.
# We could potentially fix with https://ainativedev.io/news/configuring-claude-code (writes small .claude.json file which marks onboarding as complete)

# Sync the repo into /workspace without .git or .envrc, optionally mount codex auth, then drop into a shell.
docker run --rm -it \
  "${MOUNT_ARGS[@]}" \
  "${ENV_ARGS[@]}" \
  -w /workspace \
  "${IMAGE_NAME}" \
  bash -lc 'tar -C /host --exclude=.git --exclude=.envrc -cf - . | tar -C /workspace -xf - && cd /workspace && exec bash'
