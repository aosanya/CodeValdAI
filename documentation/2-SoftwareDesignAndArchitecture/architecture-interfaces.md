```markdown
# CodeValdAI — Interfaces & Models

> Part of the split architecture. Index: [architecture.md](architecture.md)

---

## 1. AIManager Interface

`AIManager` is the sole business-logic entry point. gRPC handlers hold the
interface, never the concrete type. The implementation wraps
`entitygraph.DataManager` to expose AI-specific convenience methods.

```go
// AIManager is the primary interface for AI agent management and run execution.
// gRPC handlers hold this interface — never the concrete type.
//
// Implementations must be safe for concurrent use.
type AIManager interface {

    // ── Agent Catalogue ───────────────────────────────────────────────────────

    // CreateAgent persists a new Agent configuration.
    // Returns ErrInvalidAgent if required fields (name, provider, model,
    // system_prompt) are missing.
    // Publishes "cross.ai.{agencyID}.agent.created" after a successful write.
    CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error)

    // GetAgent retrieves a single Agent by its ID.
    // Returns ErrAgentNotFound if no agent with that ID exists.
    GetAgent(ctx context.Context, agentID string) (Agent, error)

    // ListAgents returns all Agent entities for this agency.
    ListAgents(ctx context.Context) ([]Agent, error)

    // DeleteAgent removes an Agent configuration.
    // Returns ErrAgentNotFound if no agent with that ID exists.
    // Returns ErrAgentHasActiveRuns if any run referencing this agent is
    // in pending_intake, pending_execution, or running state.
    DeleteAgent(ctx context.Context, agentID string) error

    // ── Run Lifecycle ─────────────────────────────────────────────────────────

    // IntakeRun creates an AgentRun in pending_intake state.
    // It calls the LLM with the agent's system prompt, the referenced
    // Workflow context, and the caller-supplied instructions to infer
    // what input fields are needed.
    // Returns the AgentRun (with run_id) and the inferred RunFields.
    // Returns ErrAgentNotFound if agentID does not exist.
    IntakeRun(ctx context.Context, req IntakeRunRequest) (AgentRun, []RunField, error)

    // ExecuteRun transitions a run from pending_intake to running, calls
    // the LLM with the agent system prompt + workflow context + instructions
    // + caller-supplied inputs, stores the output, and transitions to
    // completed or failed.
    // Returns ErrRunNotFound if runID does not exist.
    // Returns ErrRunNotIntaked if the run is not in pending_intake state.
    // Publishes "cross.ai.{agencyID}.run.completed" on success.
    // Publishes "cross.ai.{agencyID}.run.failed" on LLM error.
    ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error)

    // GetRun retrieves a single AgentRun by its ID.
    // Returns ErrRunNotFound if no run with that ID exists.
    GetRun(ctx context.Context, runID string) (AgentRun, error)

    // ListRuns returns all AgentRun entities matching the filter.
    ListRuns(ctx context.Context, filter RunFilter) ([]AgentRun, error)
}
```

---

## 2. LLMClient Interface

`LLMClient` is the injected abstraction for all LLM provider calls.
`cmd/main.go` constructs the desired implementation and passes it to
`NewAIManager`. The `AIManager` implementation calls it for both Intake and
Execute phases.

```go
// LLMClient abstracts LLM provider communication.
// Implementations must be safe for concurrent use.
type LLMClient interface {
    // Complete sends a completion request to the configured LLM provider
    // and returns the response. The caller is responsible for constructing
    // the full prompt (system message + user message) in CompletionRequest.
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// CompletionRequest is the provider-agnostic input to a single LLM call.
type CompletionRequest struct {
    Model       string   // e.g. "claude-3-5-sonnet-20241022"
    System      string   // System message / persona
    UserMessage string   // Full user-turn content
    Temperature float64  // Sampling temperature
    MaxTokens   int      // Max output tokens
}

// CompletionResponse is the provider-agnostic output of a single LLM call.
type CompletionResponse struct {
    Content      string // Raw text output from the LLM
    InputTokens  int    // Tokens consumed by the prompt
    OutputTokens int    // Tokens in the completion
}
```

---

## 3. AISchemaManager

Alias for `entitygraph.SchemaManager`. Used in `cmd/main.go` to seed
`DefaultAISchema()` on startup.

```go
// AISchemaManager is an alias for entitygraph.SchemaManager.
// Used only in cmd/main.go — not exposed via AIManager.
type AISchemaManager = entitygraph.SchemaManager
```

---

## 4. Data Models

### Agent

```go
// Agent is a persisted LLM configuration entity.
type Agent struct {
    ID           string  `json:"id"`
    Name         string  `json:"name"`
    Description  string  `json:"description,omitempty"`
    Provider     string  `json:"provider"`      // "anthropic" | "openai"
    Model        string  `json:"model"`         // e.g. "claude-3-5-sonnet-20241022"
    SystemPrompt string  `json:"system_prompt"`
    Temperature  float64 `json:"temperature"`   // default 0.7
    MaxTokens    int     `json:"max_tokens"`    // default 4096
    CreatedAt    string  `json:"created_at"`
    UpdatedAt    string  `json:"updated_at"`
}
```

### AgentRun

```go
// AgentRun is the execution record for a single LLM interaction.
type AgentRun struct {
    ID           string        `json:"id"`
    AgentID      string        `json:"agent_id"`
    WorkflowID   string        `json:"workflow_id,omitempty"`
    Instructions string        `json:"instructions"`
    Status       AgentRunStatus `json:"status"`
    Output       string        `json:"output,omitempty"`
    ErrorMessage string        `json:"error_message,omitempty"`
    InputTokens  int           `json:"input_tokens,omitempty"`
    OutputTokens int           `json:"output_tokens,omitempty"`
    StartedAt    string        `json:"started_at,omitempty"`
    CompletedAt  string        `json:"completed_at,omitempty"`
    CreatedAt    string        `json:"created_at"`
    UpdatedAt    string        `json:"updated_at"`
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
```

### RunField

```go
// RunField is a single input field inferred by the LLM during the Intake phase.
type RunField struct {
    ID         string   `json:"id"`
    Fieldname  string   `json:"fieldname"`
    Type       string   `json:"type"`      // "string" | "number" | "boolean" | "select" | "text"
    Label      string   `json:"label"`
    Required   bool     `json:"required"`
    Options    []string `json:"options,omitempty"` // populated when type="select"
    Ordinality int      `json:"ordinality"`
}
```

### RunInput

```go
// RunInput is a single filled value submitted by the caller during Execute.
type RunInput struct {
    ID        string `json:"id"`
    Fieldname string `json:"fieldname"`
    Value     string `json:"value"`
}
```

### Request types

```go
type CreateAgentRequest struct {
    Name         string  `json:"name"`
    Description  string  `json:"description,omitempty"`
    Provider     string  `json:"provider"`
    Model        string  `json:"model"`
    SystemPrompt string  `json:"system_prompt"`
    Temperature  float64 `json:"temperature,omitempty"`
    MaxTokens    int     `json:"max_tokens,omitempty"`
}

type IntakeRunRequest struct {
    AgentID      string `json:"agent_id"`
    WorkflowID   string `json:"workflow_id,omitempty"`
    Instructions string `json:"instructions"`
}

type RunFilter struct {
    AgentID string
    Status  AgentRunStatus
}
```
```
