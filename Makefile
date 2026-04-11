SHELL := /bin/sh

.PHONY: \
	backend-fmt backend-fmt-fix backend-lint backend-vet backend-build backend-test backend-check  \
	frontend-deps frontend-typecheck frontend-build  frontend-test frontend-check \
	check test e2e load-test up down install-hooks

backend-fmt:
	cd backend && test -z "$$(gofmt -l .)" || { \
		echo "The following backend files are not formatted properly:"; \
		gofmt -l .; \
		echo "Run 'make backend-fmt-fix' and commit the result."; \
		exit 1; \
	}

backend-fmt-fix:
	cd backend && gofmt -w $$(find . -name '*.go' -print)

backend-vet:
	cd backend && go vet ./...

backend-lint:
	cd backend && golangci-lint run ./...

backend-build:
	cd backend && go build ./cmd/server

backend-test:
	cd backend && go test -v -race ./...

backend-check:
	$(MAKE) backend-fmt
	$(MAKE) backend-vet
	$(MAKE) backend-lint
	$(MAKE) backend-build
	$(MAKE) backend-test

frontend-deps:
	cd frontend && pnpm install --frozen-lockfile

frontend-typecheck:
	cd frontend && pnpm run typecheck

frontend-build:
	cd frontend && pnpm run build

frontend-test:
	cd frontend && pnpm test

frontend-check:
	$(MAKE) frontend-typecheck
	$(MAKE) frontend-build
	$(MAKE) frontend-test

check:
	$(MAKE) backend-check
	$(MAKE) frontend-check

test:
	$(MAKE) backend-test
	$(MAKE) frontend-test

e2e:
	./scripts/compose-smoke.sh

load-test:
	./scripts/compose-load-test.sh

up:
	docker compose up --build

down:
	docker compose down -v

install-hooks:
	mkdir -p .git/hooks
	cp .githooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
