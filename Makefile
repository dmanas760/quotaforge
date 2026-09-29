.PHONY: build test test-race lint migrate run load-test docker-up docker-down

BINARY     = quotaforge
GO         = go
GOFLAGS    = -ldflags="-w -s"

## Build the binary
build:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY) ./cmd/api

## Run all tests
test:
	$(GO) test ./... -count=1 -timeout=120s

## Run tests with race detector
test-race:
	$(GO) test -race ./... -count=1 -timeout=120s

## Run vet
vet:
	$(GO) vet ./...

## Lint with golangci-lint
lint:
	golangci-lint run ./...

## Apply database migrations (requires running Postgres)
migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

## Run the API locally (requires Postgres + Redis running)
run: build
	./bin/$(BINARY)

## Start dependencies with Docker Compose
docker-up:
	docker compose up -d postgres redis

## Stop all Docker Compose services
docker-down:
	docker compose down

## Start the full stack (API included)
stack-up:
	docker compose up -d --build

## Run k6 load test
load-test:
	k6 run load/k6/scenarios.js

## Run integration tests (requires running deps)
integration-test:
	$(GO) test ./tests/integration/... -v -count=1 -timeout=300s

## Clean build artifacts
clean:
	rm -rf bin/
