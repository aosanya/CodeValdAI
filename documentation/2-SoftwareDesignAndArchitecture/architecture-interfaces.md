# CodeValdAI — Interfaces & Models

> Part of the split architecture. Index: [architecture.md](architecture.md)

---

## 1. AIManager Interface

`AIManager` is the sole business-logic entry point. gRPC handlers hold the
interface, never the concrete type. The implementation wraps
`entitygraph.DataManager` and dispatches LLM calls by reading `LLMProvider`
entities directly from the graph — no injected `LLMClient` dependency.

`cmd/main.go` wires only `entitygraph.DataManager`, `AISchemaManager`, and
`CrossPublisher` — exactly the same pattern as CodeValdAgency.

```go
// AIManager is the primary interface for AI agent management and run execution.
// gRPC handlers hold this interface — never the concrete type.
//
// Implementations must be safe for concurrent use.
type AIManager interface {

    // ── Provider Catalogue ────────────────────────────────────────────────────

    // CreateProvider persists a new LLMProvider configuration.
    // Returns ErrInvalidProvider if required fields (name, provider_type, api_key)
    // are missing or provider_type is not a known value.
    CreateProvider(ctx context.Context, req CreateProviderRequest) (LLMProvider, error)

    // GetProvider retrieves a single LLMProvider by its ID.
    // Returns ErrProviderNotFound if no provider with that ID exists.
    GetProvider(ctx context.Context, providerID string) (LLMProvider, error)

    // ListProviders returns all LLMProvider entities for this agency.
    ListProviders(ctx context.Context) ([]LLMProvider, error)

    // UpdateProvider replaces mutable fields on an LLMProvider.
    // Returns ErrProviderNotFound if no provider with that ID exists.
    UpdateProvider(ctx context.Context, providerID string, req UpdateProviderRequest) (LLMProvider, error)

    // DeleteProvider removes an LLMProvider configuration.
    // Returns ErrProviderNotFound if no provider with that ID exists.
    // Returns ErrProviderInUse if any Agent references this provider.
    DeleteProvider(ctx context.Context, providerID string) error

    // ── Agent Catalogue ───────────────────────────────────────────────────────

    // CreateAgent persists a new Agent configuration.
    // Returns ErrInvalidAgent if required fields (name, model, system_prompt)
    // are missing, or if providerID does not exist.
    // Publishes "cross.ai.{agencyID}.agent.created" after a successful write.
    CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error)

    // GetAgent retrieves a single Agent by its ID.
    // Returns ErrAgentNotFound if no agent with that ID exists.
    GetAgent(ctx context.Context, agentID string) (Agent, error)

    // ListAgents returns all Agent entities for this agency.
    ListAgents(ctx context.Context) ([]Agent, error)

    // UpdateAgent replaces mutable fields on an Agent.
    // Returns ErrAgentNotFound if no agent with that ID exists.
    UpdateAgent(ctx context.Context, agentID string, req UpdateAgentRequest) (Agent, error)

    // DeleteAgent removes an Agent configuration.
    // Returns ErrAgentNotFound if no agent with that ID exists.
    // Returns ErrAgentHasActiveRuns if any run is in pending_intake,
    // pending_execution, or running state.
    DeleteAgent(ctx context.Context, agentID string) error

    // ── Run Lifecycle ─────────────────────────────────────────────────────────

    // IntakeRun creates an AgentRun in pending_intake state.
    // Reads the Agent and its linked LLMProvider from the graph, calls the
    // LLM to infer required input fields, and stores the AgentRun + RunFields.
    // Returns the AgentRun (with run_id) and the inferred RunFields.
    // Returns ErrAgentNotFound if agentID does not exist.
    // Returns ErrProviderNotFound if the agent's linked provider does not exist.
    IntakeRun(ctx context.Context, req IntakeRunRequest) (AgentRun, []RunField, error)

    // ExecuteRun transitions a run from pending_intake to running, calls
    // the LLM with the agent's system prompt + instructions + submitted inputs,
    // stores the output, and transitions to completed or failed.
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

## 2. AISchemaManager

Alias for `entitygraph.SchemaManager`. Used in `cmd/main.go` to seed
`DefaultAISchema()` on startup.

```go
// AISchemaManager is an alias for entitygraph.SchemaManager.
// Used only in cmd/main.go — not exposed via AIManager.
type AISchemaManager = entitygraph.SchemaManager
```

---

## 3. LLM Dispatch (internal)

There is no injected `LLMClient` interface. The `aiManager` implementation
reads the `LLMProvider` entity from the graph at call time and dispatches
the HTTP request via an unexported switch. Three provider types are
supported, with `openai` and `huggingface` sharing one OpenAI-compatible
dispatcher:

```go
const defaultLLMCallTimeout = 5 * time.Minute

// internal to aiManager — not exported
func (m *aiManager) callLLM(
    ctx context.Context,
    provider LLMProvider,
    agent Agent,
    system, user string,
    onChunk func(string),    // buffer for unary RPCs, forward for streaming
) (inputTok, outputTok int, err error) {

    timeout := defaultLLMCallTimeout
    if agent.TimeoutSeconds > 0 {
        timeout = time.Duration(agent.TimeoutSeconds) * time.Second
    }
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    switch provider.ProviderType {
    case "anthropic":
        return callAnthropic(ctx, provider, agent, system, user, onChunk)
    case "openai", "huggingface":
        return callOpenAICompatible(ctx, provider, agent, system, user, onChunk)
    default:
        return 0, 0, fmt.Errorf("unsupported provider_type %q", provider.ProviderType)
    }
}
```

`callAnthropic` and `callOpenAICompatible` are unexported package-level
functions. They use `net/http` directly — no third-party SDK. Both always
send `stream: true` to the provider; `onChunk` is invoked once per
streamed token group.

The full per-task spec, per-provider request/response shapes (incl.
DeepSeek V4 via HuggingFace Router), the timeout contract, and the
startup `ReconcileRunningRuns` boot sweep are documented in
[../3-SofwareDevelopment/mvp-details/llm-client/](../3-SofwareDevelopment/mvp-details/llm-client/README.md).

---

## 4. Data Models

### LLMProvider

```go
// LLMProvider is a persisted LLM configuration entity.
// It is shared across Agents — multiple Agents may use_provider the same LLMProvider.
type LLMProvider struct {
    ID            string `json:"id"`
    Name          string `json:"name"`
    ProviderType  string `json:"provider_type"`             // "anthropic" | "openai" | "huggingface"
    APIKey        string `json:"api_key"`
    BaseURL       string `json:"base_url,omitempty"`        // empty = use provider default
    ProviderRoute string `json:"provider_route,omitempty"`  // HuggingFace-only: backend pin (e.g. "fireworks-ai")
    CreatedAt     string `json:"created_at"`
    UpdatedAt     string `json:"updated_at"`
}
```

### Agent

```go
// Agent is a persisted LLM agent configuration entity.
type Agent struct {
    ID             string  `json:"id"`
    Name           string  `json:"name"`
    Description    string  `json:"description,omitempty"`
    Model          string  `json:"model"`
    SystemPrompt   string  `json:"system_prompt"`
    Temperature    float64 `json:"temperature,omitempty"`
    MaxTokens      int     `json:"max_tokens,omitempty"`
    TimeoutSeconds int     `json:"timeout_seconds,omitempty"` // 0 = use system default (5 min)
    ProviderID     string  `json:"provider_id"`                // resolved from uses_provider edge
    CreatedAt      string  `json:"created_at"`
    UpdatedAt      string  `json:"updated_at"`
}
```

### AgentRun

```go
// AgentRun is the execution record for a single LLM interaction.
type AgentRun struct {
    ID           string         `json:"id"`
    AgentID      string         `json:"agent_id"`     // resolved from belongs_to_agent edge
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
```

### RunField

```go
// RunField is a single input field inferred by the LLM during the Intake phase.
// Immutable — written once at Intake.
type RunField struct {
    ID         string   `json:"id"`
    Fieldname  string   `json:"fieldname"`
    Type       string   `json:"type"`     // "string" | "number" | "boolean" | "select" | "text"
    Label      string   `json:"label"`
    Required   bool     `json:"required"`
    Options    []string `json:"options,omitempty"` // populated when type="select"
    Ordinality int      `json:"ordinality"`
}
```

### RunInput

```go
// RunInput is a single filled value submitted by the caller during Execute.
// Immutable — written once at Execute.
type RunInput struct {
    ID        string `json:"id"`
    Fieldname string `json:"fieldname"`
    Value     string `json:"value"`
}
```

### Request types

```go
type CreateProviderRequest struct {
    Name         string `json:"name"`
    ProviderType string `json:"provider_type"`
    APIKey       string `json:"api_key"`
    BaseURL      string `json:"base_url,omitempty"`
}

type UpdateProviderRequest struct {
    Name    string `json:"name,omitempty"`
    APIKey  string `json:"api_key,omitempty"`
    BaseURL string `json:"base_url,omitempty"`
}

type CreateAgentRequest struct {
    Name         string  `json:"name"`
    Description  string  `json:"description,omitempty"`
    ProviderID   string  `json:"provider_id"`
    Model        string  `json:"model"`
    SystemPrompt string  `json:"system_prompt"`
    Temperature  float64 `json:"temperature,omitempty"`
    MaxTokens    int     `json:"max_tokens,omitempty"`
}

type UpdateAgentRequest struct {
    Name         string  `json:"name,omitempty"`
    Description  string  `json:"description,omitempty"`
    ProviderID   string  `json:"provider_id,omitempty"`
    Model        string  `json:"model,omitempty"`
    SystemPrompt string  `json:"system_prompt,omitempty"`
    Temperature  float64 `json:"temperature,omitempty"`
    MaxTokens    int     `json:"max_tokens,omitempty"`
}

type IntakeRunRequest struct {
    AgentID      string `json:"agent_id"`
    Instructions string `json:"instructions"`
}

type RunFilter struct {
    AgentID string
    Status  AgentRunStatus
}
```
