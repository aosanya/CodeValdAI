package codevaldai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ExecuteRun implements AIManager.ExecuteRun.
// Thin wrapper around ExecuteRunStreaming with a no-op onChunk callback.
func (m *aiManager) ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error) {
	return m.ExecuteRunStreaming(ctx, runID, inputs, func(string) {})
}

// ExecuteRunStreaming implements AIManager.ExecuteRunStreaming.
// Phase 2 of the two-phase run lifecycle: validates the run is in
// pending_intake, stores submitted inputs, calls the LLM with the agent's
// system prompt and the filled input fields, then transitions to completed or
// failed. onChunk is invoked once per streamed token group; the accumulated
// chunks are also stored as AgentRun.Output.
//
// Returns ErrRunNotFound if runID does not exist.
// Returns ErrRunNotIntaked if the run is not in pending_intake state.
// Returns ErrProviderNotFound if the agent's linked provider does not exist.
// Publishes "cross.ai.{agencyID}.run.completed" on success.
// Publishes "cross.ai.{agencyID}.run.failed" on LLM error.
func (m *aiManager) ExecuteRunStreaming(ctx context.Context, runID string, inputs []RunInput, onChunk func(string)) (AgentRun, error) {
	runEntity, err := m.dm.GetEntity(ctx, m.agencyID, runID)
	if err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: %w", runID, toRunErr(err))
	}
	run := agentRunFromEntity(runEntity)

	if run.Status != AgentRunStatusPendingIntake {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: %w", runID, ErrRunNotIntaked)
	}

	for _, input := range inputs {
		if _, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
			AgencyID: m.agencyID,
			TypeID:   "RunInput",
			Properties: map[string]any{
				"fieldname": input.Fieldname,
				"value":     input.Value,
			},
			Relationships: []entitygraph.EntityRelationshipRequest{
				{Name: "belongs_to_run", ToID: runID},
			},
		}); err != nil {
			return AgentRun{}, fmt.Errorf("ExecuteRun %s: create input: %w", runID, err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":     string(AgentRunStatusPendingExecution),
			"updated_at": now,
		},
	}); err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: transition to pending_execution: %w", runID, err)
	}

	agent, provider, err := m.resolveAgentAndProvider(ctx, runID)
	if err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: %w", runID, err)
	}

	fieldRels, _ := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID,
		FromID:   runID,
		Name:     "has_field",
	})
	var runFields []RunField
	for _, rel := range fieldRels {
		fe, err := m.dm.GetEntity(ctx, m.agencyID, rel.ToID)
		if err != nil {
			continue
		}
		runFields = append(runFields, runFieldFromEntity(fe))
	}

	userMsg := buildExecuteUserMessage(run.Instructions, runFields, inputs)

	now = time.Now().UTC().Format(time.RFC3339)
	if _, err := m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":     string(AgentRunStatusRunning),
			"started_at": now,
			"updated_at": now,
		},
	}); err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: transition to running: %w", runID, err)
	}

	// Accumulate output for DB storage while also forwarding chunks to the caller.
	var output strings.Builder
	wrapped := func(s string) {
		output.WriteString(s)
		onChunk(s)
	}
	inputTok, outputTok, llmErr := m.callLLM(ctx, provider, agent, agent.SystemPrompt, userMsg, wrapped)

	now = time.Now().UTC().Format(time.RFC3339)
	if llmErr != nil {
		errMsg := llmErr.Error()
		if errors.Is(llmErr, context.DeadlineExceeded) {
			timeout := defaultLLMCallTimeout
			if agent.TimeoutSeconds > 0 {
				timeout = time.Duration(agent.TimeoutSeconds) * time.Second
			}
			errMsg = fmt.Sprintf("timeout exceeded after %s", timeout)
		}
		m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{ //nolint:errcheck
			Properties: map[string]any{
				"status":        string(AgentRunStatusFailed),
				"error_message": errMsg,
				"output":        output.String(),
				"completed_at":  now,
				"updated_at":    now,
			},
		})
		m.publish(ctx, fmt.Sprintf("cross.ai.%s.run.failed", m.agencyID))
		failedEntity, _ := m.dm.GetEntity(ctx, m.agencyID, runID)
		failed := agentRunFromEntity(failedEntity)
		failed.AgentID = agent.ID
		return failed, fmt.Errorf("ExecuteRun %s: %w", runID, llmErr)
	}

	updated, err := m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":        string(AgentRunStatusCompleted),
			"output":        output.String(),
			"input_tokens":  inputTok,
			"output_tokens": outputTok,
			"completed_at":  now,
			"updated_at":    now,
		},
	})
	if err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: store output: %w", runID, err)
	}
	m.publish(ctx, fmt.Sprintf("cross.ai.%s.run.completed", m.agencyID))

	completed := agentRunFromEntity(updated)
	completed.AgentID = agent.ID
	return completed, nil
}

// resolveAgentAndProvider follows the belongs_to_agent → uses_provider edges
// from the given run to return its Agent and LLMProvider.
func (m *aiManager) resolveAgentAndProvider(ctx context.Context, runID string) (Agent, LLMProvider, error) {
	agentRels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID,
		FromID:   runID,
		Name:     "belongs_to_agent",
	})
	if err != nil || len(agentRels) == 0 {
		return Agent{}, LLMProvider{}, ErrAgentNotFound
	}

	agentEntity, err := m.dm.GetEntity(ctx, m.agencyID, agentRels[0].ToID)
	if err != nil {
		return Agent{}, LLMProvider{}, toAgentErr(err)
	}
	agent := agentFromEntity(agentEntity)

	providerRels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID,
		FromID:   agentEntity.ID,
		Name:     "uses_provider",
	})
	if err != nil || len(providerRels) == 0 {
		return Agent{}, LLMProvider{}, ErrProviderNotFound
	}

	providerEntity, err := m.dm.GetEntity(ctx, m.agencyID, providerRels[0].ToID)
	if err != nil {
		return Agent{}, LLMProvider{}, toProviderErr(err)
	}
	return agent, providerFromEntity(providerEntity), nil
}

// buildExecuteUserMessage constructs the user-facing LLM prompt for the
// Execute phase. Each input is paired with its RunField label where available;
// unmatched inputs are included by fieldname only.
func buildExecuteUserMessage(instructions string, fields []RunField, inputs []RunInput) string {
	var sb strings.Builder
	sb.WriteString("Instructions: ")
	sb.WriteString(instructions)

	if len(inputs) > 0 {
		sb.WriteString("\nInput Fields Provided:\n")

		byFieldname := make(map[string]string, len(inputs))
		for _, inp := range inputs {
			byFieldname[inp.Fieldname] = inp.Value
		}

		for _, f := range fields {
			val, ok := byFieldname[f.Fieldname]
			if !ok {
				continue
			}
			sb.WriteString("  ")
			sb.WriteString(f.Label)
			sb.WriteString(" (")
			sb.WriteString(f.Fieldname)
			sb.WriteString("): ")
			sb.WriteString(val)
			sb.WriteString("\n")
			delete(byFieldname, f.Fieldname)
		}

		for fieldname, val := range byFieldname {
			sb.WriteString("  ")
			sb.WriteString(fieldname)
			sb.WriteString(": ")
			sb.WriteString(val)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nComplete the task.")
	return sb.String()
}
