#!/bin/sh
set -eu

STACK_SERVICES="timescaledb kafka kafka-init backend"
BASE_URL="${EXCHANGELY_E2E_BASE_URL:-http://127.0.0.1:8080}"

docker compose up -d --build $STACK_SERVICES

attempt=0
until curl -sS "$BASE_URL/api/v1/health" >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 30 ]; then
		echo "backend did not become healthy in time" >&2
		docker compose ps >&2 || true
		docker compose logs --tail=200 backend >&2 || true
		exit 1
	fi
	sleep 2
done

cd backend
EXCHANGELY_E2E_BASE_URL="$BASE_URL" go test ./tests/e2e -count=1
