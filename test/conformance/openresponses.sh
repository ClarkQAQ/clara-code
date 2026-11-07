#!/usr/bin/env bash
set -euo pipefail

readonly OPENRESPONSES_COMMIT='dce9ddb91921427c2ba06901c7dfaf0cc7056799'
readonly TCK_DIR="${OPENRESPONSES_DIR:-/tmp/clara-core-openresponses}"
readonly configured_relay_addr="${CLARA_CORE_RELAY_ADDR-${CLARY_RELAY_ADDR-}}"
readonly RELAY_ADDR="${configured_relay_addr:-127.0.0.1:18080}"

if ! command -v bun >/dev/null 2>&1; then
  echo 'bun is required' >&2
  exit 1
fi

if [[ ! -d "$TCK_DIR" ]]; then
  git clone https://github.com/openresponses/openresponses.git "$TCK_DIR"
fi

git -C "$TCK_DIR" fetch --depth=1 origin "$OPENRESPONSES_COMMIT"
git -C "$TCK_DIR" checkout --detach "$OPENRESPONSES_COMMIT"
(
  cd "$TCK_DIR"
  bun install --frozen-lockfile
)

relay_binary="$(mktemp /tmp/clara-core-relay.XXXXXX)"
go build -o "$relay_binary" ./examples/relay
CLARA_CORE_RELAY_ADDR="$RELAY_ADDR" "$relay_binary" &
relay_pid=$!
cleanup() {
  kill "$relay_pid" 2>/dev/null || true
  wait "$relay_pid" 2>/dev/null || true
  rm -f "$relay_binary"
}
trap cleanup EXIT

for _ in $(seq 1 100); do
  if curl -fs "http://$RELAY_ADDR/healthz" >/dev/null; then
    break
  fi
  sleep 0.1
done
curl -fsS "http://$RELAY_ADDR/healthz" >/dev/null

(
  cd "$TCK_DIR"
  bun run bin/compliance-test.ts \
    --base-url "http://$RELAY_ADDR/v1" \
    --api-key demo \
    --model demo \
    --json
)

