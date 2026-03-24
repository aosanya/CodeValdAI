.PHONY: build build-server run-server restart kill proto test test-arango test-all vet lint clean

export PATH := /usr/local/go/bin:$(PATH)

# ── Build ─────────────────────────────────────────────────────────────────────

## Verify the module compiles cleanly.
build:
	go build ./...

## Build the service binary to bin/codevaldai.
build-server:
	go build -o bin/codevaldai ./cmd

## Build and run the service.
## ArangoDB and Cross vars can be placed in a .env file (loaded automatically).
run-server: build-server
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	./bin/codevaldai

## Stop any running instance, rebuild, and run.
restart: kill build-server
	@echo "Running codevaldai..."
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	./bin/codevaldai

## Stop any running instances of codevaldai.
kill:
	@echo "Stopping any running instances..."
	-@pkill -9 -f "bin/codevaldai" 2>/dev/null || true
	-@fuser -k $${CODEVALDAI_GRPC_PORT:-50056}/tcp 2>/dev/null || true
	@sleep 1

# ── Proto Codegen ─────────────────────────────────────────────────────────────

## Regenerate Go stubs from proto/codevaldai/v1/*.proto.
## Requires: buf, protoc-gen-go, protoc-gen-go-grpc on PATH.
## Install: go install github.com/bufbuild/buf/cmd/buf@latest
##          go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
##          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
proto:
	buf generate

# ── Tests ─────────────────────────────────────────────────────────────────────

## Run all unit tests (integration tests skip if ArangoDB is unreachable).
test:
	go test -v -race -count=1 ./...

## Run ArangoDB integration tests.
## Loads .env if it exists, otherwise falls back to environment variables.
## Usage: make test-arango
##        CODEVALDAI_ARANGO_URL=http://host:8529 CODEVALDAI_ARANGO_USER=root CODEVALDAI_ARANGO_PASSWORD=pw make test-arango
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
