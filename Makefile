.PHONY: build build-server build-dev server dev dev-restart kill proto test cover test-integration test-arango test-all vet lint clean

export PATH := /usr/local/go/bin:$(PATH)

# ── Build ─────────────────────────────────────────────────────────────────────

## Verify the module compiles cleanly.
build:
	go build ./...

## Build the production server binary to bin/codevaldai-server.
build-server:
	go build -o bin/codevaldai-server ./cmd/server

## Build the dev binary to bin/codevaldai-dev.
build-dev:
	go build -o bin/codevaldai-dev ./cmd/dev

## Run the production server locally. Expects env vars to be set by the caller
## (or the shell) — does not source .env, to mirror container behaviour.
server: build-server
	./bin/codevaldai-server

## Run the dev binary with local-dev defaults. Sources .env if present so
## AI_ARANGO_PASSWORD etc. stay out of the source tree.
dev: build-dev
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	./bin/codevaldai-dev

## Stop any running dev instance, rebuild, and run.
dev-restart: kill dev

## Stop any running instances of the codevaldai binaries.
kill:
	@echo "Stopping any running instances..."
	-@pkill -9 -f "bin/codevaldai-" 2>/dev/null || true
	-@fuser -k $${CODEVALDAI_GRPC_PORT:-50056}/tcp 2>/dev/null || true
	@sleep 1

## Stop any running instance, rebuild, and run.
restart: dev-restart
	
	
# ── Proto Codegen ─────────────────────────────────────────────────────────────

## Regenerate Go stubs from proto/codevaldai/v1/*.proto.
## Requires: buf, protoc-gen-go, protoc-gen-go-grpc on PATH.
proto:
	buf generate

# ── Tests ─────────────────────────────────────────────────────────────────────

## Run all unit tests with race detector (skips integration tests that need ArangoDB).
test:
	go test -v -race -count=1 ./...

## Run tests and produce an HTML coverage report (coverage.html).
cover:
	go test -v -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## Run all tests including integration tests with race detector and verbose output.
test-integration:
	go test -v -race -tags integration ./...

## Run ArangoDB integration tests.
## Loads .env if it exists, otherwise falls back to environment variables.
test-arango:
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	go test -v -race -count=1 ./storage/arangodb/ ./internal/server/

## Run everything: unit tests + ArangoDB integration tests (loads .env).
test-all:
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	go test -v -race -count=1 ./...

# ── Quality ───────────────────────────────────────────────────────────────────

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	go clean ./...
	rm -rf bin/
	rm -f coverage.out coverage.html
