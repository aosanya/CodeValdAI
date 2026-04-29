# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build           # verify the module compiles
make build-server    # build production binary → bin/codevaldai-server
make dev             # build dev binary + run with .env loaded
make dev-restart     # kill any running dev instance, rebuild, and run
make kill            # stop all running instances

make test            # unit tests with race detector
make cover           # tests + HTML coverage report
make test-arango     # ArangoDB integration tests (requires .env)
make test-all        # unit + integration tests

make vet             # go vet ./...
make lint            # golangci-lint run ./...
make proto           # regenerate Go stubs from proto/ (requires buf)
make clean           # remove bin/ and coverage artefacts
```

Run a single test:
```bash
go test -v -race -run TestCreateAgent ./...
```

Integration tests require a reachable ArangoDB. Copy `.env.example` to `.env` and fill in credentials before running `make test-arango`.

## Architecture

**Three-layer design:**

```
gRPC Handlers (internal/server/)
    └─► AIManager interface (ai.go)
            ├─► entitygraph.DataManager  — ArangoDB graph CRUD (storage/arangodb/)
            ├─► callLLM dispatcher       — raw HTTP to provider APIs (dispatch.go)
            └─► CrossPublisher interface — lifecycle events to CodeValdCross (internal/registrar/)
```

`AIManager` is the single domain interface — gRPC handlers hold it, never the concrete type. `callLLM` is a method on `*aiManager` (not a separate interface); it dispatches directly over HTTP to Anthropic or OpenAI-compatible endpoints, always streaming. There is no `LLMClient` interface.

**Graph topology (ArangoDB):**

```
LLMProvider ◄──uses_provider── Agent ──has_run──► AgentRun ──has_field──► RunField
                                                            ──has_input──► RunInput
```

All five types live in the `ai_entities` collection; edges in `ai_relationships`. Inverse edges are auto-created by `entitygraph.DataManager.CreateRelationship`. The schema is seeded idempotently on startup from `DefaultAISchema()`.

**Two-phase run lifecycle:**

`IntakeRun` → creates `AgentRun` in `pending_intake`, calls LLM to infer `RunField`s.  
`ExecuteRun` → rejects anything not in `pending_intake`, stores `RunInput`s, calls LLM with full context, transitions `running → completed | failed`.

Status constants live in `models.go` (`AgentRunStatusPendingIntake` etc.). Raw string status writes are forbidden.

**Key files:**

| File | Purpose |
|---|---|
| `ai.go` | `AIManager` interface, `aiManager` concrete type, Provider+Agent CRUD, shared entity converters |
| `intake.go` | `IntakeRun` + intake-specific helpers (`parseIntakeFields`, `marshalOptions`, etc.) |
| `execute.go` | `ExecuteRun` + `buildExecuteUserMessage` |
| `dispatch.go` | `callLLM` — Anthropic SSE + OpenAI-compatible SSE dispatchers |
| `models.go` | All value types and request/filter structs |
| `errors.go` | All sentinel errors (`ErrAgentNotFound`, `ErrRunNotIntaked`, …) |
| `schema.go` | `DefaultAISchema()` — entity/edge type definitions seeded on startup |
| `internal/server/server.go` | gRPC handler — translates proto ↔ domain, calls `AIManager` |
| `internal/server/errors.go` | Domain sentinel → gRPC status code mapping |
| `internal/registrar/registrar.go` | Cross heartbeat + `CrossPublisher` implementation |
| `storage/arangodb/` | ArangoDB-backed `entitygraph.DataManager` with `ai_` prefixed collections |

## Critical Invariants

- **`AIManager` is an interface** — gRPC handlers never touch the concrete `aiManager` type.
- **No LLM SDK imports in domain code** — all LLM calls go through `callLLM`; Anthropic/OpenAI SDKs must not be imported outside `dispatch.go`.
- **No `grpc.Dial` inside `AIManager`** — Cross publishes go through the injected `CrossPublisher`; publish failures must not fail the originating operation (log and continue).
- **All graph reads/writes go through `entitygraph.DataManager`** — no raw AQL or ArangoDB driver calls outside `storage/arangodb/`.
- **Run status transitions use typed constants** — `AgentRunStatusPendingIntake`, `AgentRunStatusRunning`, etc. from `models.go`. Never write raw strings.
- **Context propagation** — every exported method accepts `context.Context` first; propagate it through `callLLM`.
- **File size limit**: 500 lines max per file; 50 lines max per function.
- **All exported symbols must have godoc comments**.
- **`provider_id` and `agent_id` on value types are resolved from edges at read time** — they are not stored as flat properties on `Agent` or `AgentRun` documents.

## Cross-Service Events

Topics produced: `cross.ai.{agencyID}.agent.created`, `cross.ai.{agencyID}.run.completed`, `cross.ai.{agencyID}.run.failed`.  
Topic consumed (future): `cross.agency.created`, `work.task.dispatched`.

## Configuration

See `.env.example`. Key variables: `CODEVALDAI_GRPC_PORT` (default `50056`), `CODEVALDAI_ARANGO_URL`, `CODEVALDAI_ARANGO_DB`, `CODEVALDAI_CROSS_ADDR`, `CODEVALDAI_AGENCY_ID`, `ANTHROPIC_API_KEY`.

## Testing Strategy

- **Unit tests** use an in-memory `fakeDataManager` (defined in `ai_test.go`) and an `httptest.Server` standing in for the LLM provider — there is no `FakeLLMClient`. Point `LLMProvider.BaseURL` at the test server and set `ProviderType` to `"openai"` or `"anthropic"`.
- **Integration tests** (`make test-arango`) hit a real ArangoDB; tagged `+build integration` in `storage/arangodb/`.
- Do not mock `entitygraph.DataManager` with a framework — use the `fakeDataManager` struct defined in test files.
