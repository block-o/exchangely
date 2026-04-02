SHELL := /bin/sh

.PHONY: fmt test up down

fmt:
	cd backend && gofmt -w $$(find . -name '*.go' -print)

test:
	cd backend && go test ./...

up:
	docker compose up --build

down:
	docker compose down -v
