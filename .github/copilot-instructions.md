# CodeValdAI — AI Agent Development Instructions

## Project Overview

**CodeValdAI** is the **AI agent management and execution** microservice of the
CodeVald platform. It owns two concerns:

1. **Agent Catalogue** — the persistent registry of LLM provider configurations
   and AI agent definitions (model, system prompt, parameters)
2. **Run Execution** — the two-phase lifecycle for running an agent against a
   workflow: **Intake** (LLM infers required input fields) → **Execute** (LLM
   runs with the submitted inputs)

**Core Concept**: CodeValdAI exposes a single domain interface — `AIManager` —
backed by an ArangoDB-backed `entitygraph` store. It registers itself with
[CodeValdCross](../CodeValdCross/README.md) over gRPC and publishes lifecycle
events (`agent.created`, `run.completed`, `run.failed`) to Cross's pub/sub bus.

---

## Service Architecture

> **Full architecture details live in the documentation.**
> See [`documentation/2-SoftwareDesignAndArchitecture/`](../documentation/2-SoftwareDesignAndArchitecture/) for:
> - Three-layer design (gRPC Server → AIManager → entitygraph storage / LLMClient / CrossPublisher)
> - `AIManager`, `LLMClient`, and `CrossPublisher` interface contracts
> - Graph topology — `LLMProvider`, `Agent`, `AgentRun`, `RunField`, `RunInput`
>   nodes and `uses_provider`, `belongs_to_agent`, `has_field`, `has_input` edges
> - Two-phase run lifecycle — pending_intake → pending_execution → running →
>   completed | failed
> - gRPC protobuf contract — [`proto/codevaldai/v1/ai.proto`](../proto/codevaldai/v1/ai.proto)

**Key invariants to keep in mind while coding:**

- `AIManager` is the **single** domain interface — gRPC handlers hold this
  interface, never the concrete type
- All LLM calls go through an injected `LLMClient` interface — never call an
  SDK directly inside `AIManager`
- All cross-service events go through an injected `CrossPublisher` interface —
  never dial CodeValdCross from the manager
- Storage is the shared-library `entitygraph` `DataManager` — agents/providers/
  runs are graph entities, not flat tables
- Every `AgentRun` carries a status enum from `models.go`; transitions are
  validated against the run lifecycle (no raw string statuses)

---

## Project Structure

```
/workspaces/CodeVald-AIProject/CodeValdAI/
├── cmd/
│   └── main.go                  # Binary entry point — wires dependencies
├── go.mod
├── go.sum
├── ai.go                        # AIManager interface (top-level package)
├── doc.go                       # Package godoc
├── errors.go                    # Sentinel errors (ErrAgentNotFound, ErrRunNotIntaked, …)
├── models.go                    # Value types (Agent, LLMProvider, AgentRun, RunField, …)
├── schema.go                    # DefaultAISchema — entity/edge definitions
├── schema_test.go
├── internal/
│   ├── config/                  # Env-var configuration loading
│   │   └── config.go
│   ├── registrar/               # Heartbeat registration with Cross + CrossPublisher impl
│   │   └── registrar.go
│   └── server/                  # Inbound gRPC handlers
│       ├── server.go            # AIService server (delegates to AIManager)
│       ├── entity_server.go     # EntityService server (graph CRUD passthrough)
│       └── errors.go            # Domain → gRPC status mapping
├── storage/
│   └── arangodb/                # ArangoDB-backed entitygraph backend
│       └── storage.go
├── proto/
│   └── codevaldai/v1/
│       └── ai.proto             # AIService gRPC contract
├── gen/
│   └── go/                      # Generated Go code (do not hand-edit)
├── documentation/
│   ├── 1-SoftwareRequirements/
│   ├── 2-SoftwareDesignAndArchitecture/
│   ├── 3-SofwareDevelopment/
│   └── 4-QA/
└── .github/
    ├── copilot-instructions.md
    ├── instructions/
    │   └── rules.instructions.md
    ├── prompts/
    └── workflows/
        └── ci.yml
```

---

## Developer Workflows

### Build & Run Commands

```bash
# Verify the module compiles cleanly
make build

# Build the service binary to bin/codevaldai
make build-server

# Build and run the service (loads .env automatically)
make run-server

# Stop any running instance, rebuild, and run
make restart

# Stop running instances
make kill

# Run all unit tests with race detector
make test

# Run ArangoDB integration tests (requires reachable ArangoDB)
make test-arango

# Run everything: unit + integration
make test-all

# Static analysis
make vet

# Lint
make lint

# Regenerate protobuf stubs (requires buf, protoc-gen-go, protoc-gen-go-grpc)
make proto

# Clean build artefacts
make clean
```

### Configuration (env vars — see `.env.example`)

| Variable | Purpose |
|---|---|
| `CODEVALDAI_GRPC_PORT` | gRPC listener port (default `50056`) |
| `CODEVALDAI_ARANGO_URL` | ArangoDB endpoint (default `http://localhost:8529`) |
| `CODEVALDAI_ARANGO_USER` / `CODEVALDAI_ARANGO_PASSWORD` | ArangoDB credentials |
| `CODEVALDAI_ARANGO_DB` | Database name |
| `CODEVALDAI_CROSS_ADDR` | CodeValdCross gRPC address (omit to disable Cross integration) |
| `CODEVALDAI_AGENCY_ID` | Agency ID sent in every Register heartbeat and used for schema seeding |
| `ANTHROPIC_API_KEY` | API key for the Anthropic LLM provider (MVP default) |

### Task Management Workflow

**Every task follows strict branch management:**

```bash
# 1. Create feature branch from main
git checkout -b feature/AI-XXX_description

# 2. Implement changes
# ... development work ...

# 3. Build validation before merge
go build ./...           # Must succeed
go vet ./...             # Must show 0 issues
go test -v -race ./...   # Must pass
golangci-lint run ./...  # Must pass

# 4. Merge when complete
git checkout main
git merge feature/AI-XXX_description --no-ff
git branch -d feature/AI-XXX_description
```

---

## Technology Stack

| Component | Choice | Rationale |
|---|---|---|
| Language | Go 1.25+ | Matches the rest of the CodeVald platform |
| gRPC framework | `google.golang.org/grpc` + protobuf | Typed contracts; shared `serverutil` from CodeValdSharedLib |
| Storage | ArangoDB via `entitygraph` (CodeValdSharedLib) | Graph topology fits LLMProvider ↔ Agent ↔ AgentRun relationships |
| LLM provider (MVP) | Anthropic | Pluggable via the `LLMClient` interface |
| Cross-service events | Heartbeat registrar + `CrossPublisher` (gRPC) | All inter-service messages go through CodeValdCross |
| Configuration | Environment variables (`internal/config/config.go`) | Loaded via `.env` for local dev |
| Health / Entity gRPC | Shared library `health` + `entitygraph/server` | Standardised across all CodeVald services |

---

## Code Quality Rules

### Service-Specific Rules

- **No business logic in `cmd/main.go`** — wire dependencies only; logic lives
  in the `codevaldai` package and `internal/`
- **`AIManager` is an interface** — concrete impl is unexported; gRPC handlers
  depend on the interface
- **`LLMClient` and `CrossPublisher` are interfaces** — injected via constructor;
  never instantiate concrete types inside flows
- **Run status transitions go through the lifecycle** — never write raw status
  strings; use `AgentRunStatus` constants from `models.go`
- **All public functions must have godoc comments**
- **Context propagation** — every public method takes `context.Context` as the
  first argument
- **No direct LLM SDK imports inside `AIManager`** — go through `LLMClient`
- **No direct gRPC dialing inside `AIManager`** — go through `CrossPublisher`
- **All graph reads/writes go through `entitygraph.DataManager`** — never write
  AQL or raw driver calls in domain code

### Naming Conventions

- **Package name**: `codevaldai` (top-level), `server`, `registrar`, `config`,
  `arangodb` (under `storage/`)
- **Interfaces**: noun-only, no `I` prefix — `AIManager`, `LLMClient`,
  `CrossPublisher`
- **Cross topics produced by AI**: `ai.{resource}.{event}` —
  e.g. `ai.run.completed`
- **Errors**: `Err{Subject}{Condition}` — `ErrAgentNotFound`,
  `ErrRunNotIntaked`, `ErrInvalidLLMResponse`
- **Run statuses**: lowercase with underscores — `pending_intake`,
  `pending_execution`, `running`, `completed`, `failed`
- **No abbreviations in exported names** — prefer `AgentID` over `AgID`

### File Organisation

- **Max file size**: 500 lines (prefer smaller, focused files)
- **Max function length**: 50 lines (prefer 20-30)
- **One primary concern per file** — separate provider, agent, and run flows in
  the manager implementation
- **Error types in `errors.go`** at module root — never scatter sentinels
- **Value types in `models.go`** at module root — `Agent`, `LLMProvider`,
  `AgentRun`, `RunField`, `RunInput`, requests, filters

### Anti-Patterns to Avoid

- ❌ **Calling Anthropic / OpenAI SDKs directly from `AIManager`** — always
  through `LLMClient`
- ❌ **Dialling Cross from a manager method** — publishes go through the
  injected `CrossPublisher`
- ❌ **Hardcoding Cross topic strings** — build them via the documented
  `ai.{...}` convention; centralise in one helper
- ❌ **Business logic in gRPC handlers** — `internal/server/server.go` only
  translates proto ↔ domain and forwards to `AIManager`
- ❌ **Panicking in exported functions** — return structured errors
- ❌ **Ignoring context cancellation** — long LLM calls must respect
  `ctx.Done()`
- ❌ **Storing API keys in logs** — never log `LLMProvider.APIKey` or any
  request/response that includes credentials
- ❌ **Skipping run lifecycle validation** — `ExecuteRun` must reject runs
  that are not in `pending_intake`

---

## Integration with CodeValdCross

> **Full integration contracts live in the documentation.**
> See `documentation/2-SoftwareDesignAndArchitecture/` for:
> - Heartbeat `Register` payload (service name `codevaldai`, advertise address,
>   topics produced/consumed)
> - Cross event topics produced by AI:
>   - `ai.agent.created`
>   - `ai.run.completed`
>   - `ai.run.failed`
> - Cross event topics consumed by AI: `cross.agency.created`,
>   `work.task.dispatched`
> - Schema seeding flow — `DefaultAISchema` is sent to Cross on startup via the
>   `entitygraph` schema route helpers

---

## Documentation References

- `documentation/1-SoftwareRequirements/` — functional requirements,
  stakeholders, problem definition
- `documentation/2-SoftwareDesignAndArchitecture/` — architecture, interface
  contracts, graph topology, gRPC contracts
- `documentation/3-SofwareDevelopment/mvp.md` — MVP task list and status
- `documentation/3-SofwareDevelopment/mvp-details/` — per-task implementation
  specs (scaffolding, llm-client, agent-management, run-intake, run-execution)
- `documentation/4-QA/` — testing strategy and test cases

---

## When in Doubt

1. **Check documentation first** — requirements and architecture docs are the
   source of truth
2. **Interface before implementation** — define the interface, write tests
   against it, then implement
3. **Inject dependencies** — `LLMClient`, `CrossPublisher`, and the
   `entitygraph.DataManager` are always caller-provided
4. **Run statuses are contracts** — adding a new status is a breaking change;
   update `models.go`, the lifecycle validator, and the architecture doc together
5. **Write tests for every exported function** — table-driven where possible;
   integration tests for ArangoDB live under `storage/arangodb/` and `internal/server/`
6. **Trace IDs at every boundary** — log agent IDs and run IDs at the entry
   and exit of every public method
