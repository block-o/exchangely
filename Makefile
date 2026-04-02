SHELL := /bin/sh

.PHONY: fmt test e2e up down

fmt:
	cd backend && gofmt -w $$(find . -name '*.go' -print)

test:
	cd backend && go test ./...

e2e:
	./scripts/compose-smoke.sh

up:
	docker compose up --build

down:
	docker compose down -v
