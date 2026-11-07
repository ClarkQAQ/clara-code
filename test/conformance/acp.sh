#!/usr/bin/env bash
set -euo pipefail

readonly ACP_TCK_DIR="${ACP_TCK_DIR:-/tmp/clara-core-acp-tck}"
readonly BINARY="${ACP_BINARY:-/tmp/clara-core-acp}"
readonly REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly ACP_TCK_COMMIT='b7f40fb3c003c45926547be5a1f7e2b374035815'

if ! command -v uv >/dev/null 2>&1; then
  echo 'uv is required. Install uv to run the ACP TCK.' >&2
  exit 1
fi

if [[ ! -d "$ACP_TCK_DIR" ]]; then
  git clone https://github.com/agentclientprotocol/acp-tck.git "$ACP_TCK_DIR"
fi

if [[ "$(git -C "$ACP_TCK_DIR" rev-parse HEAD)" != "$ACP_TCK_COMMIT" ]]; then
  git -C "$ACP_TCK_DIR" fetch --depth=1 origin "$ACP_TCK_COMMIT"
  git -C "$ACP_TCK_DIR" checkout --detach "$ACP_TCK_COMMIT"
fi

(
  cd "$REPO_ROOT"
  go build -o "$BINARY" ./examples/acp
)

(
  cd "$ACP_TCK_DIR"
  uv run acp-tck \
    --protocol-version 1 \
    --agent-cwd "$REPO_ROOT" \
    --timeout 30 \
    --test-timeout 120 \
    --cancel-prompt 'CANCEL_SLOW_TURN please keep working' \
    -- "$BINARY"
)
