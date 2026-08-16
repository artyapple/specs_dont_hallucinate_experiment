#!/usr/bin/env bash
set -euo pipefail

readonly FIXTURES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly BASE2_DIR="$FIXTURES_DIR/base2-codegen"
readonly POSTGRES_IMAGE="docker.io/library/postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941"
readonly POSTGRES="specs-base2-codegen-$RANDOM-$$"
readonly HTTP_PORT="$((20000 + RANDOM % 20000))"

cleanup() { docker rm -f "$POSTGRES" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run --rm \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --entrypoint bash \
  -v "$BASE2_DIR:/workspace:ro" \
  specs-export/tool-codegen:go1.26.6 -c 'make verify-generate'

docker run -d \
  --name "$POSTGRES" \
  --publish 127.0.0.1::5432 \
  --health-cmd='pg_isready -U postgres -d experiment' \
  --health-interval=1s \
  --health-timeout=3s \
  --health-retries=30 \
  -e POSTGRES_DB=experiment \
  -e POSTGRES_PASSWORD=canonical-only \
  "$POSTGRES_IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if [ "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = healthy ]; then break; fi
  sleep 1
done
test "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = healthy
readonly POSTGRES_PORT="$(docker inspect --format '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' "$POSTGRES")"
readonly DB_URL="postgres://postgres:canonical-only@127.0.0.1:$POSTGRES_PORT/experiment?sslmode=disable"

DATABASE_URL="$DB_URL" HTTP_ADDR="127.0.0.1:$HTTP_PORT" make -C "$BASE2_DIR" check
test "$(docker exec "$POSTGRES" psql --tuples-only --no-align --username postgres --dbname experiment --command 'SELECT count(*) FROM tasks')" = 0
printf 'Base 2 Codegen visible integration passed\n'
