// Package codevaldai provides AI agent management and execution for the
// CodeVald platform. Value types used by AIManager and its callers are
// defined here.
package codevaldai

// Agent is a persisted LLM configuration entity.
// It describes a single AI persona: which model to use, the system prompt,
// sampling parameters, and metadata.
type Agent struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	Provider     string  `json:"provider"`      // "anthropic" | "openai"
	Model        string  `json:"model"`         // e.g. "claude-3-5-sonnet-20241022"
	SystemPrompt string  `json:"system_prompt"` // Persona / task instructions for the LLM
	Temperature  float64 `json:"temperature"`   // Sampling temperature; default 0.7
	MaxTokens    int     `json:"max_tokens"`    // Max output tokens; default 4096
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// AgentRun is the execution record for a single LLM interaction.
// It captures inputs, output, token usage, and the full status history.
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
type RunInput struct {
	ID        string `json:"id"`
	Fieldname string `json:"fieldname"`
	Value     string `json:"value"`
}

// CreateAgentRequest carries the data required to create a new Agent.
// Name, Provider, Model, and SystemPrompt are required fields.
type CreateAgentRequest struct {
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	SystemPrompt string  `json:"system_prompt"`
	Temperature  float64 `json:"temperature,omitempty"`
	MaxTokens    int     `json:"max_tokens,omitempty"`
}

// IntakeRunRequest carries the data required to start the Intake phase of
// a two-phase run. The LLM uses the agent's system prompt, the optional
// WorkflowID context, and Instructions to infer what RunFields are needed.
type IntakeRunRequest struct {
	AgentID      string `json:"agent_id"`
	WorkflowID   string `json:"workflow_id,omitempty"`
	Instructions string `json:"instructions"`
}

// RunFilter constrains a ListRuns query. Zero values mean "no filter".
type RunFilter struct {
	AgentID string
	Status  AgentRunStatus
}
