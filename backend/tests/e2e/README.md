End-to-end tests run against a live Docker Compose stack.

Usage:

1. Start the backend dependencies and API:
   `docker compose up -d --build timescaledb kafka kafka-init backend`
2. Run the smoke suite:
   `cd backend && EXCHANGELY_E2E_BASE_URL=http://127.0.0.1:8080 go test ./tests/e2e -count=1`

Convenience wrapper from repo root:

`make e2e`

The Compose smoke flow also verifies Kafka topics and consumer groups from the live broker after the Go e2e checks pass.
