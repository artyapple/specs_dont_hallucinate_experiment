#!/usr/bin/env bash
set -euo pipefail

readonly FIXTURES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly BASE1_DIR="$FIXTURES_DIR/base1"
readonly CODEGEN_IMAGE="${CODEGEN_IMAGE:-specs-experiment-tool-codegen:go1.26.6}"
readonly POSTGRES_IMAGE="docker.io/library/postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941"
readonly POSTGRES="specs-formal-$RANDOM-$$"

cleanup() {
  docker rm -f "$POSTGRES" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --rm \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --entrypoint bash \
  -v "$FIXTURES_DIR:/workspace:ro" \
  "$CODEGEN_IMAGE" -c '
    set -euo pipefail
    oapi-codegen -generate types,chi-server -package contract /workspace/base1/api/openapi.yaml >/tmp/contract.gen.go
    sqlc compile -f /workspace/sqlc-validate.yaml
  '

docker run -d \
  --name "$POSTGRES" \
  --health-cmd='pg_isready -U postgres -d experiment' \
  --health-interval=1s \
  --health-timeout=3s \
  --health-retries=30 \
  -e POSTGRES_DB=experiment \
  -e POSTGRES_PASSWORD=validation-only \
  "$POSTGRES_IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if [ "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = healthy ]; then
    break
  fi
  sleep 1
done
test "$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES")" = healthy

docker exec -i "$POSTGRES" psql --set ON_ERROR_STOP=1 --username postgres --dbname experiment \
  <"$BASE1_DIR/db/migrations/000001_create_tasks.sql" >/dev/null

readonly SCHEMA="$(docker exec "$POSTGRES" psql --tuples-only --no-align --username postgres --dbname experiment --command \
  "SELECT column_name || ':' || data_type || ':' || is_nullable FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'tasks' ORDER BY ordinal_position")"
test "$SCHEMA" = $'id:uuid:NO\ntitle:text:NO\ncreated_at:timestamp with time zone:NO'

readonly INDEX="$(docker exec "$POSTGRES" psql --tuples-only --no-align --username postgres --dbname experiment --command \
  "SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'tasks_created_at_id_idx'")"
case "$INDEX" in
  *'USING btree (created_at, id)'*) ;;
  *) printf 'unexpected baseline index: %s\n' "$INDEX" >&2; exit 1 ;;
esac

printf 'baseline formal inputs validated\n'
