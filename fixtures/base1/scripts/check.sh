#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly LISTEN_ADDR="${HTTP_ADDR:-127.0.0.1:18080}"
readonly BASE_URL="http://$LISTEN_ADDR"
readonly TMP_DIR="$(mktemp -d)"
SERVICE_PID=""

cleanup() {
  if [ -n "$SERVICE_PID" ]; then
    kill "$SERVICE_PID" >/dev/null 2>&1 || true
    wait "$SERVICE_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

assert_validation_problem() {
  go run ./scripts/assert-validation-problem.go "$1" "$2"
}

: "${DATABASE_URL:?DATABASE_URL must point to the clean visible PostgreSQL sidecar}"
cd "$ROOT_DIR"

make validate-formal
go test ./...
go run ./cmd/migrate
go build -o "$TMP_DIR/task-service" ./cmd/task-service

HTTP_ADDR="$LISTEN_ADDR" "$TMP_DIR/task-service" >"$TMP_DIR/service.log" 2>&1 &
SERVICE_PID=$!

for _ in $(seq 1 30); do
  if curl --fail --silent --max-time 2 "$BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$SERVICE_PID" >/dev/null 2>&1; then
    command cat "$TMP_DIR/service.log" >&2
    exit 1
  fi
  sleep 1
done
curl --fail --silent --max-time 2 "$BASE_URL/healthz" >/dev/null

for case_name in blank-title unknown-field; do
  body="{\"title\":\" \"}"
  if [ "$case_name" = unknown-field ]; then body='{"title":"visible","extra":true}'; fi
  curl --silent --show-error --max-time 5 --output "$TMP_DIR/$case_name.json" \
    --write-out '%{http_code}\n%{content_type}\n' --header 'content-type: application/json' --data "$body" "$BASE_URL/tasks" \
    >"$TMP_DIR/$case_name.meta"
  status="$(sed -n '1p' "$TMP_DIR/$case_name.meta")"
  test "$status" = 400
  assert_validation_problem "$TMP_DIR/$case_name.json" "$(sed -n '2p' "$TMP_DIR/$case_name.meta")"
done

curl --silent --show-error --max-time 5 --output "$TMP_DIR/invalid-id.json" \
  --write-out '%{http_code}\n%{content_type}\n' "$BASE_URL/tasks/not-a-uuid" >"$TMP_DIR/invalid-id.meta"
status="$(sed -n '1p' "$TMP_DIR/invalid-id.meta")"
test "$status" = 400
assert_validation_problem "$TMP_DIR/invalid-id.json" "$(sed -n '2p' "$TMP_DIR/invalid-id.meta")"

status="$(curl --silent --show-error --max-time 5 \
  --output "$TMP_DIR/create.json" --write-out '%{http_code}' \
  --header 'content-type: application/json' \
  --data '{"title":"  visible task  "}' \
  "$BASE_URL/tasks")"
test "$status" = 201
grep -Eq '"title"[[:space:]]*:[[:space:]]*"visible task"' "$TMP_DIR/create.json"
grep -Eq '"createdAt"[[:space:]]*:[[:space:]]*"[^"]*\.[0-9]{6}Z"' "$TMP_DIR/create.json"

task_id="$(sed -n 's/.*"id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$TMP_DIR/create.json")"
test -n "$task_id"
printf '%s\n' "$task_id" | grep -Eq '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'

curl --fail --silent --max-time 5 "$BASE_URL/tasks/$task_id" >"$TMP_DIR/get.json"
grep -Fq "$task_id" "$TMP_DIR/get.json"

curl --fail --silent --max-time 5 "$BASE_URL/tasks" >"$TMP_DIR/list.json"
grep -Fq "$task_id" "$TMP_DIR/list.json"

status="$(curl --silent --show-error --max-time 5 \
  --output /dev/null --write-out '%{http_code}' \
  --request DELETE "$BASE_URL/tasks/$task_id")"
test "$status" = 204

printf 'visible check passed\n'
