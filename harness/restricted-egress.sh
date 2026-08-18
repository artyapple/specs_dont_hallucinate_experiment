#!/usr/bin/env bash
# Shared Docker topology for coordinator provider-only egress. Source this file,
# call restricted_egress_names, then restricted_egress_start.

readonly RESTRICTED_EGRESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly RESTRICTED_EGRESS_ROOT="$(cd "$RESTRICTED_EGRESS_DIR/.." && pwd)"
readonly EGRESS_PROXY_IMAGE="${EGRESS_PROXY_IMAGE:-$(jq -er '.baseImages.go' "$RESTRICTED_EGRESS_ROOT/config/versions.json")}"

restricted_egress_names() {
  local run_id="$1"
  TOOL_NETWORK="$run_id-tool"
  PROVIDER_NETWORK="$run_id-provider"
  EGRESS_PROXY="$run_id-egress-proxy"
  export TOOL_NETWORK PROVIDER_NETWORK EGRESS_PROXY
}

restricted_egress_start() {
  test -n "${TOOL_NETWORK:-}" && test -n "${PROVIDER_NETWORK:-}" && test -n "${EGRESS_PROXY:-}"
  docker network create --internal "$TOOL_NETWORK" >/dev/null
  docker network create --internal "$PROVIDER_NETWORK" >/dev/null
  docker run -d \
    --name "$EGRESS_PROXY" \
    --hostname egress-proxy \
    --network "$PROVIDER_NETWORK" \
    --network-alias egress-proxy \
    --read-only \
    --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
    --user 10001:10001 \
    --cap-drop ALL \
    --cap-add NET_BIND_SERVICE \
    --security-opt no-new-privileges \
    -e GOCACHE=/tmp/go-build \
    -e GOPROXY=off \
    -v "$RESTRICTED_EGRESS_DIR/egressproxy:/src:ro" \
    -w /src \
    --entrypoint /usr/local/go/bin/go \
    "$EGRESS_PROXY_IMAGE" run . -listen 0.0.0.0:443 -health 0.0.0.0:8080 -upstream openrouter.ai:443 >/dev/null
  docker network connect --gw-priority 1 bridge "$EGRESS_PROXY"
  EGRESS_PROXY_IP="$(docker inspect --format "{{with index .NetworkSettings.Networks \"$PROVIDER_NETWORK\"}}{{.IPAddress}}{{end}}" "$EGRESS_PROXY")"
  test -n "$EGRESS_PROXY_IP"
  export EGRESS_PROXY_IP
}

restricted_egress_stop() {
  docker rm -f "${EGRESS_PROXY:-}" >/dev/null 2>&1 || true
  docker network rm "${PROVIDER_NETWORK:-}" "${TOOL_NETWORK:-}" >/dev/null 2>&1 || true
}
