#!/bin/sh
set -eu

STACK_SERVICES="timescaledb kafka kafka-init backend"
BASE_URL="${EXCHANGELY_E2E_BASE_URL:-http://127.0.0.1:8080}"
DATABASE_URL="${EXCHANGELY_E2E_DATABASE_URL:-postgres://postgres:postgres@127.0.0.1:5432/exchangely?sslmode=disable}"
KAFKA_BROKERS="${EXCHANGELY_E2E_KAFKA_BROKERS:-127.0.0.1:9092}"
MARKET_TOPIC="${EXCHANGELY_E2E_KAFKA_MARKET_TOPIC:-exchangely.market.ticks}"
KAFKA_CONTAINER="${EXCHANGELY_E2E_KAFKA_CONTAINER:-exchangely-kafka}"

assert_topic() {
  topic="$1"
  if ! docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list | grep -Fx "$topic" >/dev/null; then
    echo "expected kafka topic $topic" >&2
    exit 1
  fi
}

assert_group() {
  group="$1"
  if ! docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list | grep -Fx "$group" >/dev/null; then
    echo "expected kafka consumer group $group" >&2
    exit 1
  fi
}

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
EXCHANGELY_E2E_BASE_URL="$BASE_URL" \
EXCHANGELY_E2E_DATABASE_URL="$DATABASE_URL" \
EXCHANGELY_E2E_KAFKA_BROKERS="$KAFKA_BROKERS" \
EXCHANGELY_E2E_KAFKA_MARKET_TOPIC="$MARKET_TOPIC" \
EXCHANGELY_E2E_KAFKA_CONTAINER="$KAFKA_CONTAINER" \
go test ./tests/e2e -count=1

assert_topic "exchangely.tasks"
assert_topic "exchangely.market.ticks"
assert_group "exchangely-workers-tasks"
assert_group "exchangely-workers-market"
