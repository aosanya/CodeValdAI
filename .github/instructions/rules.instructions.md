---
applyTo: '**'
---

# CodeValdAI — Code Structure Rules

## Service Design Principles

CodeValdAI is a **Go service** — not a library. These rules reflect that:

- **Has a `cmd/main.go` binary entry point** — wires all dependencies and starts the server
- **No business logic in `cmd/`** — `main.go` only constructs dependencies and calls `serverutil.RunWithGracefulShutdown`
- **Callers inject dependencies** — `LLMClient`, `CrossPublisher`, and the `entitygraph.DataManager` are never hardcoded
- **Exported API surface is minimal** — the top-level `codevaldai` package exposes the `AIManager` interface, value types, sentinel errors, and `DefaultAISchema`

---

## Interface-First Design

**Always define interfaces before concrete types.**

```go
// ✅ CORRECT — AIManager is an interface; the concrete impl is unexported
type AIManager interface {
    CreateProvider(ctx context.Context, req CreateProviderRequest) (LLMProvider, error)
    CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error)
    IntakeRun(ctx context.Context, req IntakeRunRequest) (AgentRun, []RunField, error)
    ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error)
}

// ❌ WRONG — leaking a concrete struct that bundles storage + LLM + publisher
type AIService struct {
    arango  *arangodb.Driver
    anthropic *anthropic.Client
    crossConn *grpc.ClientConn
}
```

**File layout — one primary concern per file:**

```
ai.go                              → AIManager interface
doc.go                             → Package godoc
errors.go                          → Sentinel errors (ErrAgentNotFound, ErrRunNotIntaked, …)
models.go                          → Value types and request/filter structs
schema.go                          → DefaultAISchema — entity & edge definitions
internal/server/server.go          → AIService gRPC handler (delegates to AIManager)
internal/server/entity_server.go   → EntityService passthrough handler
internal/server/errors.go          → Domain → gRPC status code mapping
internal/registrar/registrar.go    → Heartbeat to Cross + CrossPublisher implementation
internal/config/config.go          → Env-var configuration loading
storage/arangodb/storage.go        → ArangoDB-backed entitygraph backend
proto/codevaldai/v1/ai.proto       → gRPC contract
```

---

## LLM Client Rules

**All LLM calls go through the injected `LLMClient` interface — never call an SDK directly inside the manager.**

```go
// ✅ CORRECT — manager depends on LLMClient interface
type aiManager struct {
    data  entitygraph.DataManager
    llm   LLMClient
    cross CrossPublisher
}

// ❌ WRONG — Anthropic SDK imported and instantiated inside the flow
import "github.com/anthropics/anthropic-sdk-go"

func (m *aiManager) ExecuteRun(ctx context.Context, runID string, ...) (AgentRun, error) {
    client := anthropic.NewClient(...) // never do this
    ...
}
```

**LLM responses must be validated:**

```go
// ✅ CORRECT — validate the LLM response shape and return ErrInvalidLLMResponse
fields, err := parseRunFields(llmResp.Body)
if err != nil {
    return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: %w", agentID, ErrInvalidLLMResponse)
}

// ❌ WRONG — assume the LLM returned well-formed JSON
fields := mustParseFields(llmResp.Body) // panics on bad input
```

**Never log API keys or full LLM payloads at info level** — credentials and
prompts may contain sensitive data. Log run IDs, agent IDs, token counts, and
status transitions; redact everything else.

---

## Cross Publisher Rules

**Cross-service events go through an injected `CrossPublisher` interface — never dial Cross from the manager.**

```go
// ✅ CORRECT — manager depends on CrossPublisher interface
func (m *aiManager) ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error) {
    // ... run execution ...
    if err := m.cross.PublishRunCompleted(ctx, run); err != nil {
        // log and continue — Cross unavailability must not fail the run
        log.Printf("ExecuteRun %s: cross publish failed: %v", runID, err)
    }
    return run, nil
}

// ❌ WRONG — manager dialling Cross directly
conn, _ := grpc.Dial(crossAddr, ...)
client := pb.NewOrchestratorServiceClient(conn)
client.Publish(ctx, ...)
```

**Topic naming for events produced by CodeValdAI:**

```
cross.ai.{agencyID}.{resource}.{event}

cross.ai.{agencyID}.agent.created
cross.ai.{agencyID}.run.completed
cross.ai.{agencyID}.run.failed
```

**Cross publish failures must NOT fail the originating operation.** A successful
agent creation or run completion is durable in ArangoDB; the Cross event is a
best-effort notification. Log the failure with the run/agent ID and continue.

---

## Storage Rules

**All graph reads and writes go through the shared-library `entitygraph.DataManager` — never write AQL or use the ArangoDB driver inline.**

```go
// ✅ CORRECT — domain code uses DataManager
agent, err := m.data.GetEntity(ctx, agencyID, "agent", agentID)

// ❌ WRONG — raw AQL inside domain code
cursor, err := db.Query(ctx, "FOR a IN agents FILTER a._key == @id RETURN a", ...)
```

**Resolve linked entities through edges, never store FKs as flat properties.**

- `Agent.ProviderID` is read by following the `uses_provider` edge from agent → provider
- `AgentRun.AgentID` is read by following the `belongs_to_agent` edge
- `RunField` and `RunInput` belong to a run via `has_field` / `has_input`

The flat `*ID` fields on the value types are populated at read time from the
edges — they are not the source of truth.

**Schema seeding** — `DefaultAISchema` is the source of truth for entity/edge
shapes. Seed it idempotently on startup. Do not let domain code register entity
types ad-hoc.

---

## Run Lifecycle Rules

**`AgentRunStatus` transitions are validated — never write raw status strings.**

```go
// ✅ CORRECT — typed enum from models.go
const (
    AgentRunStatusPendingIntake    AgentRunStatus = "pending_intake"
    AgentRunStatusPendingExecution AgentRunStatus = "pending_execution"
    AgentRunStatusRunning          AgentRunStatus = "running"
    AgentRunStatusCompleted        AgentRunStatus = "completed"
    AgentRunStatusFailed           AgentRunStatus = "failed"
)

// ❌ WRONG — raw string status
run.Status = "completed" // skip the enum, lose type safety
```

**Lifecycle invariants:**

- `IntakeRun` creates a run in `pending_intake` and writes inferred `RunField`s
- `ExecuteRun` rejects any run not in `pending_intake` with `ErrRunNotIntaked`
- `ExecuteRun` transitions `pending_intake` → `running` → `completed` | `failed`
- Terminal states (`completed`, `failed`) are immutable — never mutate a run
  back into a non-terminal state
- `DeleteAgent` rejects agents with non-terminal runs via `ErrAgentHasActiveRuns`

---

## Error Handling Rules

**All exported errors are sentinels declared in `errors.go`:**

```go
// errors.go (module root)

var (
    ErrProviderNotFound   = errors.New("llm provider not found")
    ErrProviderInUse      = errors.New("llm provider is in use by one or more agents")
    ErrInvalidProvider    = errors.New("invalid provider: missing required fields or unsupported type")
    ErrAgentNotFound      = errors.New("agent not found")
    ErrAgentHasActiveRuns = errors.New("agent has active runs")
    ErrInvalidAgent       = errors.New("invalid agent: missing required fields")
    ErrRunNotFound        = errors.New("agent run not found")
    ErrRunNotIntaked      = errors.New("run is not in pending_intake state")
    ErrInvalidRunStatus   = errors.New("invalid run status transition")
    ErrInvalidLLMResponse = errors.New("invalid LLM response format")
)
```

- **Never use `log.Fatal`** in package code — return errors to the caller
- **Never panic** in exported functions
- **Wrap errors with flow + key IDs**:
  `fmt.Errorf("ExecuteRun %s: %w", runID, err)`
- **gRPC mapping** lives in `internal/server/errors.go` — sentinels translate
  to `codes.NotFound`, `codes.FailedPrecondition`, `codes.InvalidArgument`, etc.
  Do **not** `return status.Error(...)` from inside `AIManager`.

---

## Context Rules

**Every exported method must accept `context.Context` as the first argument.**

```go
// ✅ CORRECT
func (m *aiManager) ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error)

// ❌ WRONG
func (m *aiManager) ExecuteRun(runID string, inputs []RunInput) (AgentRun, error)
```

LLM calls can be slow — always:
- Propagate the caller's `ctx` to `LLMClient`
- Check `ctx.Err()` after the LLM call returns and before transitioning state

```go
resp, err := m.llm.Complete(ctx, req)
if ctx.Err() != nil {
    return AgentRun{}, ctx.Err()
}
```

---

## Godoc Rules

**Every exported type, function, interface, and method must have a godoc comment.**

```go
// ExecuteRun transitions a run from pending_intake to running, calls the
// LLM with the agent's system prompt + instructions + submitted inputs,
// stores the output, and transitions to completed or failed.
// Returns ErrRunNotFound if runID does not exist.
// Returns ErrRunNotIntaked if the run is not in pending_intake state.
// Publishes "cross.ai.{agencyID}.run.completed" on success.
func (m *aiManager) ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error) {
```

- **Package comment** on the primary file of every package (already present on
  `ai.go`, `doc.go`, `errors.go`, `models.go`, `schema.go`)
- **Examples** in `_test.go` files for the major flows (CreateAgent,
  IntakeRun, ExecuteRun)

---

## File Size and Complexity Limits

- **Max file size**: 500 lines (hard limit)
- **Max function length**: 50 lines (prefer 20-30)
- **One primary concern per file**

When the manager implementation grows, split by concern (provider catalogue,
agent catalogue, run lifecycle) rather than packing everything into one file.

---

## Concurrency Rules

- The concrete `AIManager` implementation must be safe for concurrent use —
  multiple gRPC handlers will call it in parallel
- The `entitygraph.DataManager` is the source of locking; do not add a mutex
  around per-run state inside the manager
- `IntakeRun` and `ExecuteRun` must be idempotent at the storage layer — a
  retried request with the same run ID must not duplicate `RunField` rows
- LLM calls are slow; never hold a lock across a network call

---

## Naming Conventions

```go
// ✅ CORRECT
package codevaldai

type AIManager interface{}
type LLMClient interface{}
type CrossPublisher interface{}
var ErrAgentNotFound = errors.New("agent not found")
type AgentRunStatus string

// ❌ WRONG
package codevaldais            // plural
type IAIManager interface{}    // I prefix
var agentNotFoundError = ...   // unexported sentinel exposed via behaviour
```

---

## No Direct LLM SDK Imports in Domain Code

```go
// ✅ CORRECT — call the LLM through the LLMClient interface
resp, err := m.llm.Complete(ctx, req)

// ❌ WRONG — importing the Anthropic SDK in ai.go / models.go / a manager file
import "github.com/anthropics/anthropic-sdk-go"
client := anthropic.NewClient(...)
```

The Anthropic SDK is allowed only in the concrete `LLMClient` implementation
(e.g. `internal/llm/anthropic/client.go`). Domain code is provider-agnostic.

---

## Task Management and Workflow

### Branch Management (MANDATORY)

```bash
# Create feature branch from main
git checkout -b feature/AI-XXX_description

# Implement and validate
go build ./...           # must succeed
go test -v -race ./...   # must pass
go vet ./...             # must show 0 issues
golangci-lint run ./...  # must pass

# Merge when complete
git checkout main
git merge feature/AI-XXX_description --no-ff
git branch -d feature/AI-XXX_description
```

### Pre-Development Checklist

Before adding new code:
1. ✅ Is this type already defined in `models.go` or `errors.go`?
2. ✅ Am I adding logic to the right layer (`AIManager` impl vs `internal/server` vs `storage/arangodb`)?
3. ✅ Does this function accept `context.Context` as its first argument?
4. ✅ Will the file exceed 500 lines after this change?
5. ✅ Am I injecting `LLMClient`, `CrossPublisher`, and `DataManager` instead of hardcoding?
6. ✅ Does every new exported symbol have a godoc comment?
7. ✅ Are run status transitions going through `AgentRunStatus` constants?
8. ✅ Does this flow publish the correct `cross.ai.{agencyID}.{...}` event on success?

### Code Review Requirements

Every PR must verify:
- [ ] No direct imports of LLM SDKs in domain code (only inside the `LLMClient` impl)
- [ ] No `grpc.Dial` inside `AIManager` — Cross publishes go through `CrossPublisher`
- [ ] No raw AQL or ArangoDB driver calls outside `storage/arangodb/`
- [ ] All `AgentRunStatus` writes use the typed constants from `models.go`
- [ ] All exported symbols have godoc comments
- [ ] Context propagated through all public calls, including LLM calls
- [ ] Errors are typed (`Err…`) and wrapped with flow + ID context
- [ ] No files exceeding 500 lines
- [ ] Tests added for every new exported method, with success and error cases
- [ ] `go vet ./...` shows 0 issues
- [ ] `go test -race ./...` passes
- [ ] No API keys or full LLM payloads logged
