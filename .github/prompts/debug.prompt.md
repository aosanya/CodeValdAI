---
agent: agent
---

# Debug Session

## Purpose

Guide a focused debug session for a failing test, broken build, or runtime error
in **CodeValdAI** — the AI agent management and execution service.

---

## Debug Workflow

### Step 1: Reproduce

```bash
cd /workspaces/CodeVald-AIProject/CodeValdAI

# Build
go build -o bin/codevaldai ./cmd/... 2>&1

# Specific failing test
go test -v -race ./internal/server/... -run TestXxx 2>&1

# All unit tests
go test -v -race ./... 2>&1

# ArangoDB integration tests (requires reachable Arango)
make test-arango 2>&1

# Vet
go vet ./... 2>&1
```

### Step 2: Locate the Failure

Common failure categories in CodeValdAI:

| Symptom | Likely Cause | Where to Look |
|---|---|---|
| `nil pointer dereference` in a manager call | `LLMClient` / `CrossPublisher` / `DataManager` not injected | `cmd/main.go` wiring, `NewAIManager` constructor args |
| `agent not found` / `run not found` | Edge not written; wrong agencyID in lookup | `storage/arangodb/storage.go`, edge creation in flows |
| `run is not in pending_intake state` | Caller invoked `ExecuteRun` on a fresh run, or run already terminal | `models.go` status constants, `ExecuteRun` precondition check |
| `invalid LLM response format` | LLM returned unparseable JSON during Intake | LLM prompt template, `parseRunFields` parser, retry logic |
| `context deadline exceeded` on LLM call | Caller's ctx too tight, or LLMClient not honouring `ctx` | LLM client implementation, gRPC call deadlines |
| `grpc: connection refused` on Cross publish | Cross not running / `CODEVALDAI_CROSS_ADDR` wrong | `internal/config/config.go`, registrar setup |
| Cross publish failure fails the run | `CrossPublisher` error treated as fatal | Manager flows must log + continue, not return the error |
| Schema seed errors at startup | `DefaultAISchema` mismatched with deployed schema | `schema.go`, schema seeding in `cmd/main.go` |
| Race detector hits on `aiManager` | Concurrent gRPC handlers sharing mutable state without lock | Manager fields, run lifecycle transitions |

### Step 3: Isolate

```bash
# Run only the failing package
go test -v -race ./internal/server/... -run TestExecuteRun

# Run with verbose gRPC logging
GRPC_GO_LOG_VERBOSITY_LEVEL=99 GRPC_GO_LOG_SEVERITY_LEVEL=info go test -v ./internal/server/...

# Check for data races explicitly
go test -race -count=3 ./...

# Inspect ArangoDB state during a failing integration test
# (look at the ai_entities and ai_relationships collections in the configured DB)
```

### Step 4: Fix and Re-Validate

After fixing:

```bash
go build ./...           # must succeed
go vet ./...             # must show 0 issues
go test -v -race ./...   # must pass
golangci-lint run ./...  # must pass
```

---

## Anti-Patterns That Cause Bugs

- ❌ **Calling an LLM SDK directly inside `AIManager`** — must go through `LLMClient`
- ❌ **Dialling Cross from a manager method** — publishes go through `CrossPublisher`
- ❌ **Treating Cross publish errors as fatal** — log and continue; the run is already durable
- ❌ **Writing raw `AgentRunStatus` strings** — use the typed constants from `models.go`
- ❌ **Skipping the `pending_intake` precondition in `ExecuteRun`** — must return `ErrRunNotIntaked`
- ❌ **Not propagating `ctx` to LLM calls** — long calls won't cancel on shutdown
- ❌ **Logging API keys or full LLM payloads** — credential leak, PII risk
- ❌ **Mutating a terminal run** (completed/failed) — terminal states are immutable
