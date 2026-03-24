```markdown
# CodeValdAI — Architecture

> **Split docs** — this file is the index. Details live in focused companion files:
> - [architecture-interfaces.md](architecture-interfaces.md) — `AIManager`, `LLMClient`, data models
> - [architecture-graph.md](architecture-graph.md) — graph topology, entity types, pre-delivered schema
> - [architecture-storage.md](architecture-storage.md) — ArangoDB collections, document shapes, indexes
> - [architecture-flows.md](architecture-flows.md) — run lifecycle, Intake/Execute flows, error types, gRPC service

---

## 1. Core Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Business-logic entry point | `AIManager` interface (wraps `entitygraph.DataManager`) | gRPC handlers delegate to it; convenience facade over graph operations; matches CodeValdAgency pattern |
| LLM abstraction | `LLMClient` injected interface | Provider-agnostic; Anthropic is MVP implementation; swappable without changing `AIManager` |
| Run persistence | Full `AgentRun` entity (inputs, output, token counts, timestamps) | Audit trail; optimise later if needed |
| Two-phase execution | Intake creates run in `pending_intake`; Execute fills inputs and drives to `completed`/`failed` | Caller learns what the agent needs before committing to a full LLM call |
| Storage injection | `entitygraph.DataManager` + `AISchemaManager` injected by `cmd/main.go` | Backend-agnostic; testable with fake `DataManager` |
| Downstream communication | gRPC only — no direct Go imports of other CodeVald services | Stable, versioned contracts |
| Cross registration | `OrchestratorService.Register` on startup + heartbeat every 20 s | Standard CodeVald onboarding pattern |
| Pre-delivered schema | `DefaultAISchema()` seeded on startup (idempotent) | All TypeDefinitions for Agent, AgentRun, RunField, RunInput |
| Error types | `errors.go` at module root | All exported errors in one place |
| Value types | `models.go` at module root | Pure data structs; no methods |

---

## 2. Package Structure

```
CodeValdAI/
├── cmd/
│   └── main.go                  # Wires dependencies; seeds schema; reads agencyID at startup
├── go.mod
├── errors.go                    # All exported error types
├── models.go                    # Agent, AgentRun, AgentRunStatus, RunField, RunInput, request types
├── ai.go                        # AIManager interface + aiManager implementation
├── schema.go                    # DefaultAISchema() — pre-delivered TypeDefinitions
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct + loader (env / YAML)
│   ├── llm/
│   │   ├── client.go            # LLMClient interface, CompletionRequest, CompletionResponse
│   │   └── anthropic/
│   │       └── client.go        # Anthropic implementation of LLMClient
│   ├── registrar/
│   │   └── registrar.go         # Cross registration heartbeat loop + CrossPublisher impl
│   └── server/
│       ├── server.go            # Inbound gRPC server — AIService handlers
│       ├── entity_server.go     # EntityService handlers — delegates to entitygraph.DataManager
│       └── errors.go            # gRPC status code mapping
├── storage/
│   └── arangodb/
│       ├── storage.go           # Config, Backend struct, constructors, ensureCollection
│       ├── docs.go              # ArangoDB document types and domain↔document conversions
│       └── ops.go               # Backend interface method implementations
├── proto/
│   └── codevaldai/
│       └── v1/
│           └── ai.proto         # AIService gRPC definition
├── gen/
│   └── go/                      # Generated protobuf code (buf generate — do not hand-edit)
└── bin/
    └── codevaldai               # Compiled binary
```

---

> Detailed specifications:
> - **Interfaces & models** → [architecture-interfaces.md](architecture-interfaces.md)
> - **Graph topology & schema** → [architecture-graph.md](architecture-graph.md)
> - **ArangoDB storage** → [architecture-storage.md](architecture-storage.md)
> - **Lifecycle, flows & errors** → [architecture-flows.md](architecture-flows.md)
```
