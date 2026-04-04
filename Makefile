SHELL := /bin/sh

.PHONY: \
	fmt fmt-check \
	backend-build backend-fmt backend-fmt-check backend-test backend-vet backend-lint \
	frontend-build frontend-check frontend-install frontend-test frontend-typecheck \
	check test e2e up down install-hooks

fmt:
	$(MAKE) backend-fmt

fmt-check:
	$(MAKE) backend-fmt-check

backend-build:
	cd backend && go build ./cmd/server

backend-fmt:
	cd backend && gofmt -w $$(find . -name '*.go' -print)

backend-fmt-check:
	cd backend && test -z "$$(gofmt -l .)" || { \
		echo "The following backend files are not formatted properly:"; \
		gofmt -l .; \
		echo "Run 'make fmt' and commit the result."; \
		exit 1; \
	}

backend-test:
	cd backend && go test ./...

backend-vet:
	cd backend && go vet ./...

backend-lint:
	cd backend && golangci-lint run ./...

backend-check:
	$(MAKE) backend-fmt-check
	$(MAKE) backend-vet
	$(MAKE) backend-build
	$(MAKE) backend-lint
	$(MAKE) backend-test

frontend-install:
	cd frontend && npm ci

frontend-typecheck:
	cd frontend && npm run typecheck

frontend-test:
	cd frontend && npm test

frontend-build:
	cd frontend && npm run build

frontend-check:
	$(MAKE) frontend-typecheck
	$(MAKE) frontend-test
	$(MAKE) frontend-build

check:
	$(MAKE) backend-check
	$(MAKE) frontend-check

test:
	$(MAKE) backend-test
	$(MAKE) frontend-test

e2e:
	./scripts/compose-smoke.sh

up:
	docker compose up --build

down:
	docker compose down -v

install-hooks:
	mkdir -p .git/hooks
	cp .githooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
