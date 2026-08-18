#!/usr/bin/env bash
set -euo pipefail

readonly NETWORK="specs-policy-$RANDOM-$$"
readonly TOOL="${NETWORK}-tool"
readonly COORDINATOR="${NETWORK}-coordinator"
readonly POSTGRES="${NETWORK}-postgres"
readonly SECRET="synthetic-provider-secret-$RANDOM-$$"
readonly POSTGRES_IMAGE="docker.io/library/postgres@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941"
readonly TOOL_IMAGE="${TOOL_IMAGE:-specs-experiment-tool-direct:go1.26.6}"
readonly COORDINATOR_IMAGE="${COORDINATOR_IMAGE:-specs-experiment-coordinator:1.18.18}"

cleanup() {
  docker rm -f "$TOOL" "$COORDINATOR" "$POSTGRES" >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create --internal "$NETWORK" >/dev/null

docker run -d \
  --name "$POSTGRES" \
  --network "$NETWORK" \
  --network-alias postgres \
  --health-cmd='pg_isready -U postgres' \
  --health-interval=1s \
  --health-timeout=3s \
  --health-retries=30 \
  -e POSTGRES_PASSWORD=probe \
  "$POSTGRES_IMAGE" >/dev/null

docker run -d \
  --name "$COORDINATOR" \
  --network "$NETWORK" \
  --entrypoint sleep \
  -e OPENROUTER_API_KEY="$SECRET" \
  "$COORDINATOR_IMAGE" infinity >/dev/null

docker run -d \
  --name "$TOOL" \
  --init \
  --network "$NETWORK" \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --tmpfs /home/candidate/.cache/go-build:rw,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  "$TOOL_IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if [ "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = "healthy" ]; then
    break
  fi
  sleep 1
done
test "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = "healthy"

docker exec "$TOOL" bash -c 'test "$(go version)" = "go version go1.26.6 linux/amd64"'
docker exec "$TOOL" bash -c 'test ! -e /usr/local/bin/oapi-codegen && test ! -e /usr/local/bin/sqlc'
docker exec "$TOOL" bash -c 'test ! -S /var/run/docker.sock'
docker exec "$TOOL" bash -c '! env | grep -F "synthetic-provider-secret"'
docker exec "$TOOL" bash -c '! tr "\0" "\n" </proc/1/environ | grep -F "synthetic-provider-secret"'
docker exec "$TOOL" bash -c 'for file in /proc/[0-9]*/cmdline; do if [ -r "$file" ] && grep -a -F "synthetic-provider-secret" "$file" >/dev/null; then exit 1; fi; done'
docker exec "$TOOL" bash -c 'exec 3<>/dev/tcp/postgres/5432'
docker exec "$TOOL" bash -c '! getent ahosts example.com >/dev/null 2>&1'
docker exec "$TOOL" bash -c '! curl --fail --silent --show-error --max-time 3 https://example.com/'
docker exec "$TOOL" bash -c '! curl --fail --silent --show-error --max-time 3 https://proxy.golang.org/'
docker exec "$TOOL" bash -c 'test "$(go env GOPROXY)" = off'
docker exec "$TOOL" bash -c '! command -v wget >/dev/null || ! wget -q -T 3 -O /dev/null https://example.com/'
docker exec "$TOOL" bash -c 'bash -c '\''test "$(go env GOPROXY)" = off && ! curl --fail --silent --max-time 3 https://example.com/'\'''
docker exec "$TOOL" bash -c 'test ! -w /usr/local/bin && test ! -w /workspace'

docker exec "$TOOL" bash -c 'cat >/tmp/redirect.go <<'\''EOF'\''
package main
import "net/http"
func main() { http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://example.com/", http.StatusFound) }); _ = http.ListenAndServe("127.0.0.1:18080", nil) }
EOF'
docker exec -d "$TOOL" go run /tmp/redirect.go
for _ in $(seq 1 30); do
  if docker exec "$TOOL" curl --silent --max-time 1 http://127.0.0.1:18080/ >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$TOOL" bash -c '! curl --location --fail --silent --show-error --max-time 3 http://127.0.0.1:18080/'
