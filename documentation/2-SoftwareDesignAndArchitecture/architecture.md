# CodeValdAI — Architecture

> **Split docs** — this file is the index. Details live in focused companion files:
> - [architecture-interfaces.md](architecture-interfaces.md) — `AIManager`, data models, LLM dispatch
> - [architecture-graph.md](architecture-graph.md) — graph topology, entity types, pre-delivered schema
> - [architecture-storage.md](architecture-storage.md) — ArangoDB collections, document shapes, indexes
> - [architecture-flows.md](architecture-flows.md) — run lifecycle, Intake/Execute flows, error types

---

## 1. Core Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Business-logic entry point | `AIManager` interface (wraps `entitygraph.DataManager`) | gRPC handlers delegate to it; convenience facade over graph operations; matches CodeValdAgency pattern |
| LLM provider | `LLMProvider` is a first-class graph entity (not an injected interface) | Data-driven; matches how CodeValdAgency stores `ConfiguredRole`; no code deploy needed to add a provider |
| LLM dispatch | Unexported `callLLM` switch in `ai.go` on `provider.ProviderType` | `callAnthropic` is an unexported package function; no third-party SDK |
| API key storage | `api_key` stored directly in ArangoDB on the `LLMProvider` entity | Simple; owner controls access at the DB/API layer |
| Run persistence | Full `AgentRun` entity (instructions, output, token counts, timestamps) | Audit trail; optimise later if needed |
| Two-phase execution | Intake creates run in `pending_intake`; Execute fills inputs and drives to `completed`/`failed` | Caller learns what the agent needs before committing to a full LLM call |
| Storage injection | `entitygraph.DataManager` + `AISchemaManager` injected by `cmd/main.go` | Backend-agnostic; testable with fake `DataManager` |
| Downstream communication | gRPC only — no direct Go imports of other CodeVald services | Stable, versioned contracts |
| Cross registration | `OrchestratorService.Register` on startup + heartbeat every 20 s | Standard CodeVald onboarding pattern |
| Pre-delivered schema | `DefaultAISchema()` seeded on startup (idempotent) | TypeDefinitions for LLMProvider, Agent, AgentRun, RunField, RunInput |
| EntityService gRPC handler | `egserver.NewEntityServer` from SharedLib `entitygraph/server` | Same pattern as CodeValdAgency; no AI-specific handler code |
| Schema seed | `entitygraph.SeedSchema` from SharedLib | Idempotent startup helper shared across all services |
| Entity gRPC route path | `egserver.GRPCServicePath` (`/entitygraph.v1.EntityService`) | Constant from SharedLib; used when advertising entity HTTP routes to Cross |
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
├── models.go                    # LLMProvider, Agent, AgentRun, AgentRunStatus, RunField, RunInput, request types
├── ai.go                        # AIManager interface + aiManager implementation (includes callLLM dispatch)
├── schema.go                    # DefaultAISchema() — pre-delivered TypeDefinitions
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct + loader (env / YAML)
│   ├── registrar/
│   │   └── registrar.go         # Cross registration heartbeat loop + CrossPublisher impl
│   └── server/
│       ├── server.go            # Inbound gRPC server — AIService handlers
│       ├── entity_server.go     # Re-export of egserver.NewEntityServer from SharedLib (same pattern as CodeValdAgency)
│       └── errors.go            # AIService-domain gRPC error mapping
├── storage/
│   └── arangodb/
│       └── storage.go           # Config, Backend struct, ArangoDB implementation (thin wrapper over entitygraph)
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
