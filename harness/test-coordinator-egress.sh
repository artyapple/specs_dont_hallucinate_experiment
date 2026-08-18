#!/usr/bin/env bash
# Model-free validation of the exact coordinator network topology. This sends
# one unauthenticated HTTPS request to OpenRouter but never invokes OpenCode.
set -euo pipefail

readonly HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=restricted-egress.sh
. "$HARNESS_DIR/restricted-egress.sh"

readonly RUN_ID="specs-egress-$RANDOM-$$"
readonly TOOL="$RUN_ID-tool"
readonly COORDINATOR="$RUN_ID-coordinator"
readonly TOOL_IMAGE="${TOOL_IMAGE:-specs-experiment-tool-direct:go1.26.6}"
readonly COORDINATOR_IMAGE="${COORDINATOR_IMAGE:-specs-experiment-coordinator:1.18.18}"
readonly WORKSPACE="$(mktemp -d)"
restricted_egress_names "$RUN_ID"

cleanup() {
  docker rm -f "$TOOL" "$COORDINATOR" >/dev/null 2>&1 || true
  restricted_egress_stop
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT

fail() {
  printf 'coordinator egress test failed: %s\n' "$1" >&2
  exit 1
}

if [ "${OPENROUTER_API_KEY+x}" = x ]; then
  fail "OPENROUTER_API_KEY must be absent"
fi

restricted_egress_start

docker run -d \
  --name "$TOOL" \
  --init \
  --hostname tool-executor \
  --network "$TOOL_NETWORK" \
  --network-alias tool \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --tmpfs /home/candidate/.cache/go-build:rw,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -v "$WORKSPACE:/workspace" \
  "$TOOL_IMAGE" >/dev/null

docker create \
  --name "$COORDINATOR" \
  --hostname coordinator \
  --network "$TOOL_NETWORK" \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --add-host "openrouter.ai:$EGRESS_PROXY_IP" \
  --entrypoint sleep \
  "$COORDINATOR_IMAGE" infinity >/dev/null
docker network connect "$PROVIDER_NETWORK" "$COORDINATOR"
docker start "$COORDINATOR" >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$COORDINATOR" curl --fail --silent --max-time 2 http://egress-proxy:8080/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$COORDINATOR" curl --fail --silent --max-time 2 http://egress-proxy:8080/healthz >/dev/null \
  || fail "egress proxy did not become healthy"
OPENROUTER_STATUS="$(docker exec "$COORDINATOR" curl --silent --show-error --max-time 20 --output /dev/null --write-out '%{http_code}' https://openrouter.ai/)" \
  || fail "allowed OpenRouter HTTPS request failed"
test "$OPENROUTER_STATUS" != 000 || fail "OpenRouter returned no HTTP response"

for url in https://example.com/ https://proxy.golang.org/ http://openrouter.ai/; do
  if docker exec "$COORDINATOR" curl --fail --silent --show-error --max-time 5 --output /dev/null "$url" >/dev/null 2>&1; then
    fail "non-allowlisted request succeeded: $url"
  fi
done
OPENROUTER_UPSTREAM_IP="$(docker exec "$EGRESS_PROXY" sh -c 'getent ahostsv4 openrouter.ai | { read -r ip _; printf "%s" "$ip"; }')"
test -n "$OPENROUTER_UPSTREAM_IP" || fail "relay could not resolve the real OpenRouter endpoint"
if docker exec "$COORDINATOR" curl --resolve "openrouter.ai:443:$OPENROUTER_UPSTREAM_IP" --fail --silent --show-error --max-time 5 --output /dev/null https://openrouter.ai/ >/dev/null 2>&1; then
  fail "coordinator bypassed the relay through a direct upstream address"
fi

docker exec "$COORDINATOR" curl --fail --silent --max-time 2 http://tool:4096/healthz >/dev/null \
  || fail "coordinator cannot reach the internal tool bridge"
docker exec "$TOOL" bash -c '! getent hosts egress-proxy >/dev/null 2>&1' \
  || fail "tool resolved the provider proxy"
docker exec "$TOOL" bash -c '! curl --fail --silent --max-time 3 https://openrouter.ai/' \
  || fail "tool reached OpenRouter"

docker exec "$TOOL" bash -c 'cat >/tmp/redirect.go <<'\''EOF'\''
package main
import "net/http"
func main() { http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://example.com/", http.StatusFound) }); _ = http.ListenAndServe("0.0.0.0:18080", nil) }
EOF
go run /tmp/redirect.go >/tmp/redirect.log 2>&1 &'
for _ in $(seq 1 30); do
  if docker exec "$COORDINATOR" curl --silent --max-time 1 http://tool:18080/ >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if docker exec "$COORDINATOR" curl --location --fail --silent --show-error --max-time 5 http://tool:18080/ >/dev/null 2>&1; then
  fail "redirect escaped the provider allowlist"
fi

test "$(docker inspect "$COORDINATOR" | jq -r '.[0].NetworkSettings.Networks | keys | sort | join(" ")')" \
  = "$(printf '%s\n%s\n' "$PROVIDER_NETWORK" "$TOOL_NETWORK" | sort | tr '\n' ' ' | sed 's/ $//')" \
  || fail "coordinator network membership differs from the restricted topology"
test "$(docker inspect "$TOOL" | jq -r '.[0].NetworkSettings.Networks | keys | join(" ")')" = "$TOOL_NETWORK" \
  || fail "tool has an unexpected network"
docker exec "$EGRESS_PROXY" sh -c 'test -z "${OPENROUTER_API_KEY+x}"' \
  || fail "provider credential reached the egress proxy"

printf 'coordinator restricted-egress test passed\n'
