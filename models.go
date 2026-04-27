// Package codevaldai provides AI agent management and execution for the
// CodeVald platform. Value types used by AIManager and its callers are
// defined here.
package codevaldai

// LLMProvider is a persisted LLM configuration entity.
// It is shared across Agents — multiple Agents may reference the same
// LLMProvider via a uses_provider edge.
//
// ProviderRoute is HuggingFace-only: when non-empty the dispatcher appends
// it to Agent.Model as ":<route>" to pin the HuggingFace Router to a
// specific backend (e.g. "fireworks-ai"). It is ignored for the
// "anthropic" and "openai" provider types.
type LLMProvider struct {
ID            string `json:"id"`
Name          string `json:"name"`
ProviderType  string `json:"provider_type"` // "anthropic" | "openai" | "huggingface"
APIKey        string `json:"api_key"`
BaseURL       string `json:"base_url,omitempty"`       // empty = use provider default endpoint
ProviderRoute string `json:"provider_route,omitempty"` // HuggingFace-only: backend pin (e.g. "fireworks-ai")
CreatedAt     string `json:"created_at"`
UpdatedAt     string `json:"updated_at"`
}

// Agent is a persisted LLM agent configuration entity.
// It references one LLMProvider via the uses_provider graph edge.
//
// TimeoutSeconds overrides the system default LLM-call timeout for this
// Agent. A zero value (or absent field) means "use the system default"
// (see [defaultLLMCallTimeout]).
type Agent struct {
ID             string  `json:"id"`
Name           string  `json:"name"`
Description    string  `json:"description,omitempty"`
ProviderID     string  `json:"provider_id"` // resolved from uses_provider edge
Model          string  `json:"model"`         // e.g. "claude-3-5-sonnet-20241022"
SystemPrompt   string  `json:"system_prompt"` // Persona / task instructions for the LLM
Temperature    float64 `json:"temperature,omitempty"`
MaxTokens      int     `json:"max_tokens,omitempty"`
TimeoutSeconds int     `json:"timeout_seconds,omitempty"` // 0 = system default
CreatedAt      string  `json:"created_at"`
UpdatedAt      string  `json:"updated_at"`
}

// AgentRun is the execution record for a single LLM interaction.
// The agent_id is resolved at read time from the belongs_to_agent edge —
// it is not stored as a flat property on the AgentRun document.
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

// AgentRunStatus enumerates the valid states of an AgentRun lifecycle.
type AgentRunStatus string

const (
// AgentRunStatusPendingIntake is the initial state after IntakeRun creates
// the run. The LLM has inferred the required input fields but the caller
// has not yet submitted them.
AgentRunStatusPendingIntake AgentRunStatus = "pending_intake"

// AgentRunStatusPendingExecution indicates that inputs have been received
// and the run is queued for the Execute phase.
AgentRunStatusPendingExecution AgentRunStatus = "pending_execution"

// AgentRunStatusRunning indicates the Execute phase is actively calling
// the LLM.
AgentRunStatusRunning AgentRunStatus = "running"

// AgentRunStatusCompleted indicates the Execute phase finished
// successfully and output is stored.
AgentRunStatusCompleted AgentRunStatus = "completed"

// AgentRunStatusFailed indicates the Execute phase encountered an
// unrecoverable error (e.g. LLM call failed).
AgentRunStatusFailed AgentRunStatus = "failed"
)

// RunField is a single input field inferred by the LLM during the Intake
// phase. The caller uses the returned RunFields to build a form and then
// submits filled RunInputs to ExecuteRun.
// RunField is immutable — written once at Intake.
type RunField struct {
ID         string   `json:"id"`
Fieldname  string   `json:"fieldname"`
Type       string   `json:"type"` // "string" | "number" | "boolean" | "select" | "text"
Label      string   `json:"label"`
Required   bool     `json:"required"`
Options    []string `json:"options,omitempty"` // populated when type="select"
Ordinality int      `json:"ordinality"`
}

// RunInput is a single filled value submitted by the caller during the
// Execute phase to satisfy a RunField inferred by Intake.
// RunInput is immutable — written once at Execute.
type RunInput struct {
ID        string `json:"id"`
Fieldname string `json:"fieldname"`
Value     string `json:"value"`
}

// CreateProviderRequest carries the data required to create a new LLMProvider.
// Name, ProviderType, and APIKey are required fields.
type CreateProviderRequest struct {
Name          string `json:"name"`
ProviderType  string `json:"provider_type"` // "anthropic" | "openai" | "huggingface"
APIKey        string `json:"api_key"`
BaseURL       string `json:"base_url,omitempty"`
ProviderRoute string `json:"provider_route,omitempty"` // HuggingFace-only
}

// UpdateProviderRequest carries the mutable fields that may be changed on
// an existing LLMProvider. Only non-empty fields are applied.
//
// ProviderType is intentionally absent: changing the type of an existing
// LLMProvider invalidates uses_provider edges that referenced it for a
// specific request shape. Callers must delete and recreate.
type UpdateProviderRequest struct {
Name          string `json:"name,omitempty"`
APIKey        string `json:"api_key,omitempty"`
BaseURL       string `json:"base_url,omitempty"`
ProviderRoute string `json:"provider_route,omitempty"`
}

// CreateAgentRequest carries the data required to create a new Agent.
// Name, ProviderID, Model, and SystemPrompt are required fields.
type CreateAgentRequest struct {
Name           string  `json:"name"`
Description    string  `json:"description,omitempty"`
ProviderID     string  `json:"provider_id"`
Model          string  `json:"model"`
SystemPrompt   string  `json:"system_prompt"`
Temperature    float64 `json:"temperature,omitempty"`
MaxTokens      int     `json:"max_tokens,omitempty"`
TimeoutSeconds int     `json:"timeout_seconds,omitempty"` // 0 = system default
}

// UpdateAgentRequest carries the mutable fields that may be changed on an
// existing Agent. Only non-empty / non-zero fields are applied.
type UpdateAgentRequest struct {
Name           string  `json:"name,omitempty"`
Description    string  `json:"description,omitempty"`
ProviderID     string  `json:"provider_id,omitempty"`
Model          string  `json:"model,omitempty"`
SystemPrompt   string  `json:"system_prompt,omitempty"`
Temperature    float64 `json:"temperature,omitempty"`
MaxTokens      int     `json:"max_tokens,omitempty"`
TimeoutSeconds int     `json:"timeout_seconds,omitempty"`
}

// IntakeRunRequest carries the data required to start the Intake phase of
// a two-phase run. The LLM uses the agent's system prompt and the caller-
// supplied Instructions to infer what RunFields are needed.
type IntakeRunRequest struct {
AgentID      string `json:"agent_id"`
Instructions string `json:"instructions"`
}

// RunFilter constrains a ListRuns query. Zero values mean "no filter".
type RunFilter struct {
AgentID string
Status  AgentRunStatus
}
