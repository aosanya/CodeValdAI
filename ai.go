// Package codevaldai provides AI agent management and execution for the
// CodeVald platform. It exposes [AIManager] — the single interface for
// creating agents, running the two-phase intake/execute lifecycle, and
// publishing run events to CodeValdCross.
package codevaldai

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdAI/internal/llm"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// AIManager is the primary interface for AI agent management and run execution.
// gRPC handlers hold this interface — never the concrete type.
//
// Implementations must be safe for concurrent use.
type AIManager interface {
	// ── Agent Catalogue ───────────────────────────────────────────────────────

	// CreateAgent persists a new Agent configuration.
	// Returns [ErrInvalidAgent] if required fields (Name, Provider, Model,
	// SystemPrompt) are missing.
	// Publishes "cross.ai.{agencyID}.agent.created" after a successful write.
	CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error)

	// GetAgent retrieves a single Agent by its ID.
	// Returns [ErrAgentNotFound] if no agent with that ID exists.
	GetAgent(ctx context.Context, agentID string) (Agent, error)

	// ListAgents returns all Agent entities for this agency.
	ListAgents(ctx context.Context) ([]Agent, error)

	// DeleteAgent removes an Agent configuration.
	// Returns [ErrAgentNotFound] if no agent with that ID exists.
	// Returns [ErrAgentHasActiveRuns] if any run referencing this agent is in
	// pending_intake, pending_execution, or running state.
	DeleteAgent(ctx context.Context, agentID string) error

	// ── Run Lifecycle ─────────────────────────────────────────────────────────

	// IntakeRun creates an AgentRun in pending_intake state.
	// It calls the LLM with the agent's system prompt, the optional workflow
	// context, and the caller-supplied Instructions to infer what input fields
	// are needed. Returns the AgentRun (with run_id) and the inferred RunFields.
	// Returns [ErrAgentNotFound] if agentID does not exist.
	IntakeRun(ctx context.Context, req IntakeRunRequest) (AgentRun, []RunField, error)

	// ExecuteRun transitions a run from pending_intake to running, calls the
	// LLM with the agent system prompt + workflow context + instructions +
	// caller-supplied inputs, stores the output, and transitions to completed
	// or failed.
	// Returns [ErrRunNotFound] if runID does not exist.
	// Returns [ErrRunNotIntaked] if the run is not in pending_intake state.
	// Publishes "cross.ai.{agencyID}.run.completed" on success.
	// Publishes "cross.ai.{agencyID}.run.failed" on LLM error.
	ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error)

	// GetRun retrieves a single AgentRun by its ID.
	// Returns [ErrRunNotFound] if no run with that ID exists.
	GetRun(ctx context.Context, runID string) (AgentRun, error)

	// ListRuns returns all AgentRun entities matching the filter.
	ListRuns(ctx context.Context, filter RunFilter) ([]AgentRun, error)
}

// AISchemaManager is a type alias for [entitygraph.SchemaManager].
// Used in cmd/main.go to seed [DefaultAISchema] on startup.
type AISchemaManager = entitygraph.SchemaManager

// CrossPublisher publishes AI lifecycle events to CodeValdCross.
// Implementations must be safe for concurrent use. A nil CrossPublisher is
// valid — publish calls are silently skipped.
type CrossPublisher interface {
	// Publish delivers an event for the given topic and agencyID to
	// CodeValdCross. Errors are non-fatal: implementations should log and
	// return nil for best-effort delivery.
	Publish(ctx context.Context, topic string, agencyID string) error
}

// aiManager is the concrete implementation of [AIManager].
// It wraps [entitygraph.DataManager] to expose AI-specific convenience
// methods. All storage operations go through dm; no bespoke Backend interface
// is used.
type aiManager struct {
	dm        entitygraph.DataManager // graph CRUD — injected by cmd/main.go
	sm        AISchemaManager         // schema versioning — injected by cmd/main.go
	llmClient llm.LLMClient           // LLM provider — injected by cmd/main.go
	publisher CrossPublisher          // optional; nil = skip event publishing
	agencyID  string                  // the single agency ID for this database
}

// NewAIManager constructs an [AIManager] backed by the given
// [entitygraph.DataManager], [AISchemaManager], and [llm.LLMClient].
// agencyID is the single agency scoped to this database; it is passed to every
// DataManager call as the scope key.
// pub may be nil — cross-service events are skipped when no publisher is set.
func NewAIManager(
	dm entitygraph.DataManager,
	sm AISchemaManager,
	llmClient llm.LLMClient,
	pub CrossPublisher,
	agencyID string,
) AIManager {
	return &aiManager{
		dm:        dm,
		sm:        sm,
		llmClient: llmClient,
		publisher: pub,
		agencyID:  agencyID,
	}
}

// ── Agent Catalogue ───────────────────────────────────────────────────────────

// CreateAgent persists a new Agent entity in the graph.
// Required fields: Name, Provider, Model, SystemPrompt.
// Publishes "cross.ai.{agencyID}.agent.created" on success.
func (m *aiManager) CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error) {
	if req.Name == "" || req.Provider == "" || req.Model == "" || req.SystemPrompt == "" {
		return Agent{}, ErrInvalidAgent
	}

	now := time.Now().UTC().Format(time.RFC3339)
	props := map[string]any{
		"name":          req.Name,
		"description":   req.Description,
		"provider":      req.Provider,
		"model":         req.Model,
		"system_prompt": req.SystemPrompt,
		"temperature":   req.Temperature,
		"max_tokens":    req.MaxTokens,
		"created_at":    now,
		"updated_at":    now,
	}

	entity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   m.agencyID,
		TypeID:     "Agent",
		Properties: props,
	})
	if err != nil {
		return Agent{}, fmt.Errorf("CreateAgent: %w", err)
	}

	agent := agentFromEntity(entity)
	m.publish(ctx, fmt.Sprintf("cross.ai.%s.agent.created", m.agencyID))
	return agent, nil
}

// GetAgent retrieves a single Agent by its entity ID.
// Returns [ErrAgentNotFound] if no matching agent exists.
func (m *aiManager) GetAgent(ctx context.Context, agentID string) (Agent, error) {
	entity, err := m.dm.GetEntity(ctx, m.agencyID, agentID)
	if err != nil {
		return Agent{}, fmt.Errorf("GetAgent %s: %w", agentID, toAgentErr(err))
	}
	return agentFromEntity(entity), nil
}

// ListAgents returns all non-deleted Agent entities for this agency.
func (m *aiManager) ListAgents(ctx context.Context) ([]Agent, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "Agent",
	})
	if err != nil {
		return nil, fmt.Errorf("ListAgents: %w", err)
	}

	agents := make([]Agent, 0, len(entities))
	for _, e := range entities {
		agents = append(agents, agentFromEntity(e))
	}
	return agents, nil
}

// DeleteAgent removes an Agent entity from the graph.
// Returns [ErrAgentNotFound] if the agent does not exist.
// Returns [ErrAgentHasActiveRuns] if any active run references this agent.
func (m *aiManager) DeleteAgent(ctx context.Context, agentID string) error {
	// Guard: check for active runs before deletion.
	activeRuns, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "AgentRun",
	})
	if err != nil {
		return fmt.Errorf("DeleteAgent %s: list runs: %w", agentID, err)
	}
	for _, r := range activeRuns {
		rAgentID, _ := r.Properties["agent_id"].(string)
		status, _ := r.Properties["status"].(string)
		if rAgentID == agentID && isActiveRunStatus(AgentRunStatus(status)) {
			return ErrAgentHasActiveRuns
		}
	}

	if err := m.dm.DeleteEntity(ctx, m.agencyID, agentID); err != nil {
		return fmt.Errorf("DeleteAgent %s: %w", agentID, toAgentErr(err))
	}
	return nil
}

// ── Run Lifecycle ─────────────────────────────────────────────────────────────

// IntakeRun implementation is provided in MVP-AI-013.
// Returns a not-implemented error until then.
func (m *aiManager) IntakeRun(_ context.Context, req IntakeRunRequest) (AgentRun, []RunField, error) {
	return AgentRun{}, nil, fmt.Errorf("IntakeRun: not implemented (MVP-AI-013)")
}

// ExecuteRun implementation is provided in MVP-AI-014.
// Returns a not-implemented error until then.
func (m *aiManager) ExecuteRun(_ context.Context, runID string, _ []RunInput) (AgentRun, error) {
	return AgentRun{}, fmt.Errorf("ExecuteRun %s: not implemented (MVP-AI-014)", runID)
}

// GetRun retrieves a single AgentRun by its entity ID.
// Returns [ErrRunNotFound] if no matching run exists.
func (m *aiManager) GetRun(ctx context.Context, runID string) (AgentRun, error) {
	entity, err := m.dm.GetEntity(ctx, m.agencyID, runID)
	if err != nil {
		return AgentRun{}, fmt.Errorf("GetRun %s: %w", runID, toRunErr(err))
	}
	return agentRunFromEntity(entity), nil
}

// ListRuns returns all AgentRun entities matching the given filter.
func (m *aiManager) ListRuns(ctx context.Context, filter RunFilter) ([]AgentRun, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "AgentRun",
	})
	if err != nil {
		return nil, fmt.Errorf("ListRuns: %w", err)
	}

	runs := make([]AgentRun, 0, len(entities))
	for _, e := range entities {
		run := agentRunFromEntity(e)
		if filter.AgentID != "" && run.AgentID != filter.AgentID {
			continue
		}
		if filter.Status != "" && run.Status != filter.Status {
			continue
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// publish delivers an event to CodeValdCross. Errors are swallowed —
// events are best-effort and must not fail the originating operation.
func (m *aiManager) publish(ctx context.Context, topic string) {
	if m.publisher == nil {
		return
	}
	_ = m.publisher.Publish(ctx, topic, m.agencyID)
}

// isActiveRunStatus reports whether the status represents a non-terminal run.
func isActiveRunStatus(s AgentRunStatus) bool {
	switch s {
	case AgentRunStatusPendingIntake, AgentRunStatusPendingExecution, AgentRunStatusRunning:
		return true
	}
	return false
}

// agentFromEntity converts an [entitygraph.Entity] to an [Agent] value type.
func agentFromEntity(e entitygraph.Entity) Agent {
	p := e.Properties
	return Agent{
		ID:           e.ID,
		Name:         strProp(p, "name"),
		Description:  strProp(p, "description"),
		Provider:     strProp(p, "provider"),
		Model:        strProp(p, "model"),
		SystemPrompt: strProp(p, "system_prompt"),
		Temperature:  float64Prop(p, "temperature"),
		MaxTokens:    intProp(p, "max_tokens"),
		CreatedAt:    strProp(p, "created_at"),
		UpdatedAt:    strProp(p, "updated_at"),
	}
}

// agentRunFromEntity converts an [entitygraph.Entity] to an [AgentRun] value type.
func agentRunFromEntity(e entitygraph.Entity) AgentRun {
	p := e.Properties
	return AgentRun{
		ID:           e.ID,
		AgentID:      strProp(p, "agent_id"),
		WorkflowID:   strProp(p, "workflow_id"),
		Instructions: strProp(p, "instructions"),
		Status:       AgentRunStatus(strProp(p, "status")),
		Output:       strProp(p, "output"),
		ErrorMessage: strProp(p, "error_message"),
		InputTokens:  intProp(p, "input_tokens"),
		OutputTokens: intProp(p, "output_tokens"),
		StartedAt:    strProp(p, "started_at"),
		CompletedAt:  strProp(p, "completed_at"),
		CreatedAt:    strProp(p, "created_at"),
		UpdatedAt:    strProp(p, "updated_at"),
	}
}

// toAgentErr maps entitygraph errors to AIManager agent errors.
func toAgentErr(err error) error {
	if err == entitygraph.ErrEntityNotFound {
		return ErrAgentNotFound
	}
	return err
}

// toRunErr maps entitygraph errors to AIManager run errors.
func toRunErr(err error) error {
	if err == entitygraph.ErrEntityNotFound {
		return ErrRunNotFound
	}
	return err
}

// strProp extracts a string property value; returns "" when absent or wrong type.
func strProp(p map[string]any, key string) string {
	v, _ := p[key].(string)
	return v
}

// float64Prop extracts a float64 property value; returns 0 when absent or wrong type.
func float64Prop(p map[string]any, key string) float64 {
	v, _ := p[key].(float64)
	return v
}

// intProp extracts an int property value; returns 0 when absent or wrong type.
func intProp(p map[string]any, key string) int {
	switch v := p[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}
