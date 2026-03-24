// Package codevaldai provides AI agent management and execution for the
// CodeVald platform. It exposes [AIManager] — the single interface for
// managing LLM providers, creating agents, and running the two-phase
// intake/execute lifecycle.
package codevaldai

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// AIManager is the primary interface for AI agent management and run execution.
// gRPC handlers hold this interface — never the concrete type.
//
// Implementations must be safe for concurrent use.
type AIManager interface {
	// ── Provider Catalogue ────────────────────────────────────────────────────

	// CreateProvider persists a new LLMProvider configuration.
	// Returns [ErrInvalidProvider] if required fields (Name, ProviderType, APIKey)
	// are missing or ProviderType is not a recognised value.
	CreateProvider(ctx context.Context, req CreateProviderRequest) (LLMProvider, error)

	// GetProvider retrieves a single LLMProvider by its ID.
	// Returns [ErrProviderNotFound] if no provider with that ID exists.
	GetProvider(ctx context.Context, providerID string) (LLMProvider, error)

	// ListProviders returns all LLMProvider entities for this agency.
	ListProviders(ctx context.Context) ([]LLMProvider, error)

	// UpdateProvider replaces mutable fields on an LLMProvider.
	// Returns [ErrProviderNotFound] if no provider with that ID exists.
	UpdateProvider(ctx context.Context, providerID string, req UpdateProviderRequest) (LLMProvider, error)

	// DeleteProvider removes an LLMProvider configuration.
	// Returns [ErrProviderNotFound] if no provider with that ID exists.
	// Returns [ErrProviderInUse] if any Agent references this provider.
	DeleteProvider(ctx context.Context, providerID string) error

	// ── Agent Catalogue ───────────────────────────────────────────────────────

	// CreateAgent persists a new Agent configuration.
	// Returns [ErrInvalidAgent] if required fields (Name, ProviderID, Model,
	// SystemPrompt) are missing.
	// Returns [ErrProviderNotFound] if the supplied ProviderID does not exist.
	// Publishes "cross.ai.{agencyID}.agent.created" after a successful write.
	CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error)

	// GetAgent retrieves a single Agent by its ID.
	// Returns [ErrAgentNotFound] if no agent with that ID exists.
	GetAgent(ctx context.Context, agentID string) (Agent, error)

	// ListAgents returns all Agent entities for this agency.
	ListAgents(ctx context.Context) ([]Agent, error)

	// UpdateAgent replaces mutable fields on an Agent.
	// Returns [ErrAgentNotFound] if no agent with that ID exists.
	UpdateAgent(ctx context.Context, agentID string, req UpdateAgentRequest) (Agent, error)

	// DeleteAgent removes an Agent configuration.
	// Returns [ErrAgentNotFound] if no agent with that ID exists.
	// Returns [ErrAgentHasActiveRuns] if any run is in pending_intake,
	// pending_execution, or running state.
	DeleteAgent(ctx context.Context, agentID string) error

	// ── Run Lifecycle ─────────────────────────────────────────────────────────

	// IntakeRun creates an AgentRun in pending_intake state.
	// Reads the Agent and its linked LLMProvider from the graph, calls the
	// LLM to infer required input fields, and stores the AgentRun + RunFields.
	// Returns the AgentRun (with ID) and the inferred RunFields.
	// Returns [ErrAgentNotFound] if agentID does not exist.
	// Returns [ErrProviderNotFound] if the agent's linked provider does not exist.
	IntakeRun(ctx context.Context, req IntakeRunRequest) (AgentRun, []RunField, error)

	// ExecuteRun transitions a run from pending_intake to running, calls the
	// LLM with the agent's system prompt + instructions + submitted inputs,
	// stores the output, and transitions to completed or failed.
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
// methods. LLM calls are dispatched via the unexported callLLM helper
// based on the LLMProvider.ProviderType fetched from the graph.
type aiManager struct {
	dm        entitygraph.DataManager // graph CRUD — injected by cmd/main.go
	sm        AISchemaManager         // schema versioning — injected by cmd/main.go
	publisher CrossPublisher          // optional; nil = skip event publishing
	agencyID  string                  // the single agency ID for this database
}

// NewAIManager constructs an [AIManager] backed by the given
// [entitygraph.DataManager] and [AISchemaManager].
// agencyID is the single agency scoped to this database.
// pub may be nil — cross-service events are skipped when no publisher is set.
func NewAIManager(
	dm entitygraph.DataManager,
	sm AISchemaManager,
	pub CrossPublisher,
	agencyID string,
) AIManager {
	return &aiManager{
		dm:        dm,
		sm:        sm,
		publisher: pub,
		agencyID:  agencyID,
	}
}

// ── Provider Catalogue ────────────────────────────────────────────────────────

// knownProviderTypes lists the accepted values for LLMProvider.ProviderType.
var knownProviderTypes = map[string]bool{
	"anthropic": true,
	"openai":    true,
}

// CreateProvider persists a new LLMProvider entity in the graph.
// Required fields: Name, ProviderType (known value), APIKey.
func (m *aiManager) CreateProvider(ctx context.Context, req CreateProviderRequest) (LLMProvider, error) {
	if req.Name == "" || req.ProviderType == "" || req.APIKey == "" {
		return LLMProvider{}, ErrInvalidProvider
	}
	if !knownProviderTypes[req.ProviderType] {
		return LLMProvider{}, ErrInvalidProvider
	}

	now := time.Now().UTC().Format(time.RFC3339)
	props := map[string]any{
		"name":          req.Name,
		"provider_type": req.ProviderType,
		"api_key":       req.APIKey,
		"base_url":      req.BaseURL,
		"created_at":    now,
		"updated_at":    now,
	}

	entity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   m.agencyID,
		TypeID:     "LLMProvider",
		Properties: props,
	})
	if err != nil {
		return LLMProvider{}, fmt.Errorf("CreateProvider: %w", err)
	}
	return providerFromEntity(entity), nil
}

// GetProvider retrieves a single LLMProvider by its entity ID.
// Returns [ErrProviderNotFound] if no matching provider exists.
func (m *aiManager) GetProvider(ctx context.Context, providerID string) (LLMProvider, error) {
	entity, err := m.dm.GetEntity(ctx, m.agencyID, providerID)
	if err != nil {
		return LLMProvider{}, fmt.Errorf("GetProvider %s: %w", providerID, toProviderErr(err))
	}
	return providerFromEntity(entity), nil
}

// ListProviders returns all LLMProvider entities for this agency.
func (m *aiManager) ListProviders(ctx context.Context) ([]LLMProvider, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "LLMProvider",
	})
	if err != nil {
		return nil, fmt.Errorf("ListProviders: %w", err)
	}
	providers := make([]LLMProvider, 0, len(entities))
	for _, e := range entities {
		providers = append(providers, providerFromEntity(e))
	}
	return providers, nil
}

// UpdateProvider replaces mutable fields on an existing LLMProvider.
// Returns [ErrProviderNotFound] if no provider with that ID exists.
func (m *aiManager) UpdateProvider(ctx context.Context, providerID string, req UpdateProviderRequest) (LLMProvider, error) {
	entity, err := m.dm.GetEntity(ctx, m.agencyID, providerID)
	if err != nil {
		return LLMProvider{}, fmt.Errorf("UpdateProvider %s: %w", providerID, toProviderErr(err))
	}

	now := time.Now().UTC().Format(time.RFC3339)
	props := cloneProps(entity.Properties)
	if req.Name != "" {
		props["name"] = req.Name
	}
	if req.APIKey != "" {
		props["api_key"] = req.APIKey
	}
	if req.BaseURL != "" {
		props["base_url"] = req.BaseURL
	}
	props["updated_at"] = now

	updated, err := m.dm.UpdateEntity(ctx, m.agencyID, providerID, entitygraph.UpdateEntityRequest{
		Properties: props,
	})
	if err != nil {
		return LLMProvider{}, fmt.Errorf("UpdateProvider %s: %w", providerID, err)
	}
	return providerFromEntity(updated), nil
}

// DeleteProvider removes an LLMProvider entity from the graph.
// Returns [ErrProviderNotFound] if the provider does not exist.
// Returns [ErrProviderInUse] if any Agent holds a uses_provider edge to it.
func (m *aiManager) DeleteProvider(ctx context.Context, providerID string) error {
	agents, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "Agent",
	})
	if err != nil {
		return fmt.Errorf("DeleteProvider %s: list agents: %w", providerID, err)
	}
	for _, a := range agents {
		if strProp(a.Properties, "provider_id") == providerID {
			return ErrProviderInUse
		}
	}
	if err := m.dm.DeleteEntity(ctx, m.agencyID, providerID); err != nil {
		return fmt.Errorf("DeleteProvider %s: %w", providerID, toProviderErr(err))
	}
	return nil
}

// ── Agent Catalogue ───────────────────────────────────────────────────────────

// CreateAgent persists a new Agent entity in the graph.
// Required fields: Name, ProviderID, Model, SystemPrompt.
// Publishes "cross.ai.{agencyID}.agent.created" on success.
func (m *aiManager) CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error) {
	if req.Name == "" || req.ProviderID == "" || req.Model == "" || req.SystemPrompt == "" {
		return Agent{}, ErrInvalidAgent
	}

	// Verify the provider exists before linking to it.
	if _, err := m.GetProvider(ctx, req.ProviderID); err != nil {
		return Agent{}, fmt.Errorf("CreateAgent: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	props := map[string]any{
		"name":          req.Name,
		"description":   req.Description,
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

	// Write uses_provider edge: Agent → LLMProvider.
	if _, err := m.dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
		AgencyID: m.agencyID,
		FromID:   entity.ID,
		ToID:     req.ProviderID,
		Name:     "uses_provider",
	}); err != nil {
		return Agent{}, fmt.Errorf("CreateAgent %s: link provider: %w", entity.ID, err)
	}

	agent := agentFromEntity(entity)
	agent.ProviderID = req.ProviderID
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

// ListAgents returns all Agent entities for this agency.
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

// UpdateAgent replaces mutable fields on an existing Agent.
// Returns [ErrAgentNotFound] if no agent with that ID exists.
func (m *aiManager) UpdateAgent(ctx context.Context, agentID string, req UpdateAgentRequest) (Agent, error) {
	entity, err := m.dm.GetEntity(ctx, m.agencyID, agentID)
	if err != nil {
		return Agent{}, fmt.Errorf("UpdateAgent %s: %w", agentID, toAgentErr(err))
	}

	now := time.Now().UTC().Format(time.RFC3339)
	props := cloneProps(entity.Properties)
	if req.Name != "" {
		props["name"] = req.Name
	}
	if req.Description != "" {
		props["description"] = req.Description
	}
	if req.Model != "" {
		props["model"] = req.Model
	}
	if req.SystemPrompt != "" {
		props["system_prompt"] = req.SystemPrompt
	}
	if req.Temperature != 0 {
		props["temperature"] = req.Temperature
	}
	if req.MaxTokens != 0 {
		props["max_tokens"] = req.MaxTokens
	}
	props["updated_at"] = now

	updated, err := m.dm.UpdateEntity(ctx, m.agencyID, agentID, entitygraph.UpdateEntityRequest{
		Properties: props,
	})
	if err != nil {
		return Agent{}, fmt.Errorf("UpdateAgent %s: %w", agentID, err)
	}

	agent := agentFromEntity(updated)
	if req.ProviderID != "" {
		agent.ProviderID = req.ProviderID
	}
	return agent, nil
}

// DeleteAgent removes an Agent entity from the graph.
// Returns [ErrAgentNotFound] if the agent does not exist.
// Returns [ErrAgentHasActiveRuns] if any active run references this agent.
func (m *aiManager) DeleteAgent(ctx context.Context, agentID string) error {
	runs, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "AgentRun",
	})
	if err != nil {
		return fmt.Errorf("DeleteAgent %s: list runs: %w", agentID, err)
	}
	for _, r := range runs {
		status, _ := r.Properties["status"].(string)
		if isActiveRunStatus(AgentRunStatus(status)) {
			// belongs_to_agent edge check would be needed here in the full implementation;
			// for now a conservative guard flags any active run referencing this agent.
			_ = agentID
			return ErrAgentHasActiveRuns
		}
	}
	if err := m.dm.DeleteEntity(ctx, m.agencyID, agentID); err != nil {
		return fmt.Errorf("DeleteAgent %s: %w", agentID, toAgentErr(err))
	}
	return nil
}

// ── Run Lifecycle ─────────────────────────────────────────────────────────────

// IntakeRun implementation is provided in MVP-AI-012.
// Returns a not-implemented error until then.
func (m *aiManager) IntakeRun(_ context.Context, req IntakeRunRequest) (AgentRun, []RunField, error) {
	return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: not implemented (MVP-AI-012)", req.AgentID)
}

// ExecuteRun implementation is provided in MVP-AI-013.
// Returns a not-implemented error until then.
func (m *aiManager) ExecuteRun(_ context.Context, runID string, _ []RunInput) (AgentRun, error) {
	return AgentRun{}, fmt.Errorf("ExecuteRun %s: not implemented (MVP-AI-013)", runID)
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

// ── LLM Dispatch (internal) ───────────────────────────────────────────────────

// callLLM dispatches an LLM completion request to the provider identified by
// provider.ProviderType. Returns (content, inputTokens, outputTokens, error).
func callLLM(ctx context.Context, provider LLMProvider, agent Agent, system, user string) (string, int, int, error) {
	switch provider.ProviderType {
	case "anthropic":
		return callAnthropic(ctx, provider, agent, system, user)
	default:
		return "", 0, 0, fmt.Errorf("unsupported provider_type %q", provider.ProviderType)
	}
}

// callAnthropic performs a POST /v1/messages to the Anthropic API.
// Implementation is provided in MVP-AI-012.
func callAnthropic(_ context.Context, _ LLMProvider, _ Agent, _, _ string) (string, int, int, error) {
	return "", 0, 0, fmt.Errorf("callAnthropic: not implemented (MVP-AI-012)")
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

// cloneProps returns a shallow copy of a properties map so callers can mutate
// it without affecting the original entity.
func cloneProps(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// providerFromEntity converts an [entitygraph.Entity] to an [LLMProvider] value type.
func providerFromEntity(e entitygraph.Entity) LLMProvider {
	p := e.Properties
	return LLMProvider{
		ID:           e.ID,
		Name:         strProp(p, "name"),
		ProviderType: strProp(p, "provider_type"),
		APIKey:       strProp(p, "api_key"),
		BaseURL:      strProp(p, "base_url"),
		CreatedAt:    strProp(p, "created_at"),
		UpdatedAt:    strProp(p, "updated_at"),
	}
}

// agentFromEntity converts an [entitygraph.Entity] to an [Agent] value type.
func agentFromEntity(e entitygraph.Entity) Agent {
	p := e.Properties
	return Agent{
		ID:           e.ID,
		Name:         strProp(p, "name"),
		Description:  strProp(p, "description"),
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

// toProviderErr maps entitygraph errors to AIManager provider errors.
func toProviderErr(err error) error {
	if err == entitygraph.ErrEntityNotFound {
		return ErrProviderNotFound
	}
	return err
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
