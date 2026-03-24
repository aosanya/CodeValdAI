````markdown
# Scaffolding & Core Types — Implementation Details

---

## MVP-AI-001 — Module Scaffolding

**Status**: 🔲 Not Started
**Branch**: `feature/AI-001_module_scaffolding`

### Goal

Bootstrap the Go module with the correct directory structure, build tooling, and proto toolchain config.
No business logic — only skeleton files and configuration.

### Files to Create

| File | Purpose |
|---|---|
| `go.mod` | `module github.com/aosanya/CodeValdAI` — Go 1.21+; add `CodeValdSharedLib` replace directive |
| `Makefile` | `build`, `run`, `test`, `vet`, `lint`, `proto` targets matching CodeValdAgency Makefile |
| `buf.yaml` | Proto lint config |
| `buf.gen.yaml` | `go` + `go-grpc` plugins pointing to `gen/go/` |
| `cmd/main.go` | Empty stub — `package main; func main() {}` |
| `proto/codevaldai/v1/.gitkeep` | Reserve proto directory |
| `gen/go/.gitkeep` | Reserve generated-code directory |
| `internal/config/.gitkeep` | Reserve config directory |
| `internal/llm/.gitkeep` | Reserve LLM client directory |
| `internal/registrar/.gitkeep` | Reserve registrar directory |
| `internal/server/.gitkeep` | Reserve server directory |
| `storage/arangodb/.gitkeep` | Reserve storage directory |
| `.env.example` | Document required env vars: `CODEVALDAI_GRPC_PORT`, `CODEVALDAI_ARANGO_URL`, `CODEVALDAI_ARANGO_DB`, `ANTHROPIC_API_KEY`, `CODEVALDAI_CROSS_ADDR`, `CODEVALDAI_AGENCY_ID` |

### Acceptance Tests

- `go build ./...` succeeds on the empty skeleton
- `go vet ./...` shows 0 issues
- All required directories exist

---

## MVP-AI-002 — Domain Models

**Status**: 🔲 Not Started
**Branch**: `feature/AI-002_domain_models`

### Goal

Define all pure data types in `models.go`. No business logic, no methods (except constants).

### File: `models.go`

```go
package codevaldai

// Agent is a persisted LLM configuration entity.
type Agent struct {
    ID           string  `json:"id"`
    Name         string  `json:"name"`
    Description  string  `json:"description,omitempty"`
    Provider     string  `json:"provider"`
    Model        string  `json:"model"`
    SystemPrompt string  `json:"system_prompt"`
    Temperature  float64 `json:"temperature"`
    MaxTokens    int     `json:"max_tokens"`
    CreatedAt    string  `json:"created_at"`
    UpdatedAt    string  `json:"updated_at"`
}

// AgentRun is the execution record for a single LLM interaction.
type AgentRun struct {
    ID           string         `json:"id"`
    AgentID      string         `json:"agent_id"`
    WorkflowID   string         `json:"workflow_id,omitempty"`
    Instructions string         `json:"instructions"`
    Status       AgentRunStatus `json:"status"`
    Output       string         `json:"output,omitempty"`
    ErrorMessage string         `json:"error_message,omitempty"`
    InputTokens  int            `json:"input_tokens,omitempty"`
    OutputTokens int            `json:"output_tokens,omitempty"`
    StartedAt    string         `json:"started_at,omitempty"`
    CompletedAt  string         `json:"completed_at,omitempty"`
    CreatedAt    string         `json:"created_at"`
    UpdatedAt    string         `json:"updated_at"`
}

// AgentRunStatus enumerates the valid run states.
type AgentRunStatus string

const (
    AgentRunStatusPendingIntake    AgentRunStatus = "pending_intake"
    AgentRunStatusPendingExecution AgentRunStatus = "pending_execution"
    AgentRunStatusRunning          AgentRunStatus = "running"
    AgentRunStatusCompleted        AgentRunStatus = "completed"
    AgentRunStatusFailed           AgentRunStatus = "failed"
)

// RunField is a single input field inferred by the LLM during the Intake phase.
type RunField struct {
    ID         string   `json:"id"`
    Fieldname  string   `json:"fieldname"`
    Type       string   `json:"type"`
    Label      string   `json:"label"`
    Required   bool     `json:"required"`
    Options    []string `json:"options,omitempty"`
    Ordinality int      `json:"ordinality"`
}

// RunInput is a single filled value submitted by the caller during Execute.
type RunInput struct {
    ID        string `json:"id"`
    Fieldname string `json:"fieldname"`
    Value     string `json:"value"`
}

// CreateAgentRequest carries the fields for a new Agent.
type CreateAgentRequest struct {
    Name         string  `json:"name"`
    Description  string  `json:"description,omitempty"`
    Provider     string  `json:"provider"`
    Model        string  `json:"model"`
    SystemPrompt string  `json:"system_prompt"`
    Temperature  float64 `json:"temperature,omitempty"`
    MaxTokens    int     `json:"max_tokens,omitempty"`
}

// IntakeRunRequest carries the fields to start the Intake phase.
type IntakeRunRequest struct {
    AgentID      string `json:"agent_id"`
    WorkflowID   string `json:"workflow_id,omitempty"`
    Instructions string `json:"instructions"`
}

// RunFilter scopes a ListRuns query.
type RunFilter struct {
    AgentID string
    Status  AgentRunStatus
}
```

### Acceptance Tests

- All types compile without errors
- Zero-value structs are valid (no required constructors)

---

## MVP-AI-003 — Error Types

**Status**: 🔲 Not Started
**Branch**: `feature/AI-003_error_types`

### Goal

Define all exported sentinel errors in `errors.go`. No scattered errors across files.

### File: `errors.go`

```go
package codevaldai

import "errors"

var (
    ErrAgentNotFound      = errors.New("agent not found")
    ErrRunNotFound        = errors.New("agent run not found")
    ErrRunNotIntaked      = errors.New("run is not in pending_intake state")
    ErrInvalidRunStatus   = errors.New("invalid run status transition")
    ErrInvalidAgent       = errors.New("invalid agent: missing required fields")
    ErrAgentHasActiveRuns = errors.New("agent has active runs")
    ErrInvalidLLMResponse = errors.New("invalid LLM response format")
)
```

### Acceptance Tests

- All errors are non-nil sentinel values
- `errors.Is` works correctly for each error

---

## MVP-AI-004 — Pre-Delivered Schema

**Status**: 🔲 Not Started
**Branch**: `feature/AI-004_pre_delivered_schema`

### Goal

Implement `DefaultAISchema()` in `schema.go`. Returns the fixed `types.Schema`
with TypeDefinitions for Agent, AgentRun, RunField, RunInput.
See [architecture-graph.md](../../2-SoftwareDesignAndArchitecture/architecture-graph.md) §4 for the full TypeDefinition specs.

### File: `schema.go`

```go
// Package codevaldai — pre-delivered schema definition.
// DefaultAISchema returns the fixed types.Schema seeded by cmd/main.go
// on startup via AISchemaManager.SetSchema.
package codevaldai

import "github.com/aosanya/CodeValdSharedLib/types"

// DefaultAISchema returns the pre-delivered schema containing TypeDefinitions
// for Agent, AgentRun, RunField, and RunInput.
// cmd/main.go seeds this schema idempotently on startup.
func DefaultAISchema() types.Schema {
    return types.Schema{
        // ... TypeDefinitions as specified in architecture-graph.md §4
    }
}
```

### Acceptance Tests

- `DefaultAISchema()` returns a non-zero `types.Schema`
- All four TypeIDs are present: `"Agent"`, `"AgentRun"`, `"RunField"`, `"RunInput"`
- Required fields on each TypeDefinition match the spec in `architecture-graph.md`

---

## MVP-AI-005 — AIManager Interface

**Status**: 🔲 Not Started
**Branch**: `feature/AI-005_aimanager_interface`

### Goal

Define the `AIManager` interface and the unexported `aiManager` concrete type in `ai.go`.
The concrete implementation body can be stubbed — methods return `nil, ErrAgentNotFound` etc.
Full implementation comes in MVP-AI-013 and MVP-AI-014.

### File: `ai.go`

Key points:
- `AIManager` interface with all 8 methods (see [architecture-interfaces.md](../../2-SoftwareDesignAndArchitecture/architecture-interfaces.md))
- `aiManager` struct: fields `dm entitygraph.DataManager`, `sm entitygraph.SchemaManager`, `llm LLMClient`, `publisher Publisher`, `agencyID string`
- `NewAIManager(dm, sm, llm, publisher, agencyID)` constructor — returns `(AIManager, error)`, validates non-nil args
- `Publisher` interface with `Publish(ctx, topic, payload string) error`

### Acceptance Tests

- `NewAIManager(nil, sm, llm, pub, id)` returns an error
- `NewAIManager(dm, nil, llm, pub, id)` returns an error
- `NewAIManager(dm, sm, nil, pub, id)` returns an error
- `NewAIManager(dm, sm, llm, pub, "")` returns an error
- Valid constructor call returns a non-nil `AIManager`
````
