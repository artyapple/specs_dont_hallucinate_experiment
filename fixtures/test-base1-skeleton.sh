#!/usr/bin/env bash
set -euo pipefail

readonly FIXTURES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly BASE1_DIR="$FIXTURES_DIR/base1"
readonly POSTGRES_IMAGE="docker.io/library/postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941"
readonly POSTGRES="specs-base1-$RANDOM-$$"
readonly TMP_DIR="$(mktemp -d)"
readonly HTTP_PORT="$((20000 + RANDOM % 20000))"
SERVICE_PID=""

cleanup() {
  if [ -n "$SERVICE_PID" ]; then
    kill "$SERVICE_PID" >/dev/null 2>&1 || true
    wait "$SERVICE_PID" >/dev/null 2>&1 || true
  fi
  docker rm -f "$POSTGRES" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

make -C "$BASE1_DIR" verify-skeleton

docker run -d \
  --name "$POSTGRES" \
  --publish 127.0.0.1::5432 \
  --health-cmd='pg_isready -U postgres -d experiment' \
  --health-interval=1s \
  --health-timeout=3s \
  --health-retries=30 \
  -e POSTGRES_DB=experiment \
  -e POSTGRES_PASSWORD=skeleton-only \
  "$POSTGRES_IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if [ "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = healthy ]; then
    break
  fi
  sleep 1
done
test "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = healthy

readonly POSTGRES_PORT="$(docker inspect --format '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' "$POSTGRES")"
readonly DB_URL="postgres://postgres:skeleton-only@127.0.0.1:$POSTGRES_PORT/experiment?sslmode=disable"

DATABASE_URL="$DB_URL" make -C "$BASE1_DIR" migrate
test "$(docker exec "$POSTGRES" psql --tuples-only --no-align --username postgres --dbname experiment --command "SELECT to_regclass('public.tasks') IS NOT NULL")" = t

go -C "$BASE1_DIR" build -o "$TMP_DIR/task-service" ./cmd/task-service
DATABASE_URL="$DB_URL" HTTP_ADDR="127.0.0.1:$HTTP_PORT" "$TMP_DIR/task-service" >"$TMP_DIR/service.log" 2>&1 &
SERVICE_PID=$!

for _ in $(seq 1 30); do
  if curl --fail --silent --max-time 2 "http://127.0.0.1:$HTTP_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl --fail --silent --max-time 2 "http://127.0.0.1:$HTTP_PORT/healthz" >/dev/null

readonly TASKS_STATUS="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 2 "http://127.0.0.1:$HTTP_PORT/tasks")"
test "$TASKS_STATUS" = 404

kill -TERM "$SERVICE_PID"
wait "$SERVICE_PID"
SERVICE_PID=""

printf 'Base 1 infrastructure skeleton validated; Task API remains absent\n'
