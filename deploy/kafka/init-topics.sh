#!/bin/sh
set -eu

BOOTSTRAP_SERVER="${BOOTSTRAP_SERVER:-kafka:9092}"

create_topic() {
  topic="$1"
  partitions="${2:-12}"

  /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "${BOOTSTRAP_SERVER}" \
    --create \
    --if-not-exists \
    --topic "${topic}" \
    --partitions "${partitions}" \
    --replication-factor 1
}

create_topic "exchangely.tasks" 12
create_topic "exchangely.market.ticks" 12
