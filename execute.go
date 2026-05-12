package codevaldai

import (
	"context"
	"errors"
	"fmt"
	"log"
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
// Publishes "ai.{agencyID}.run.completed" on success.
// Publishes "ai.{agencyID}.run.failed" on LLM error.
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

	// Build the enriched system prompt: static agent prompt + action catalogue
	// + hydrated event context (task details, etc.) fetched just-in-time.
	systemPrompt := m.buildSystemPrompt(ctx, agent.SystemPrompt, run.Instructions)

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

	log.Printf("codevaldai: ExecuteRun run=%s agent=%s provider=%s system_prompt_len=%d user_msg_len=%d",
		runID, agent.ID, provider.Name, len(systemPrompt), len(userMsg))
	m.publishJSON(ctx, TopicTaskInProgress, TaskInProgressPayload{
		TaskID:  run.TaskID,
		RunID:   runID,
		AgentID: agent.ID,
	})

	// Accumulate output for DB storage while also forwarding chunks to the caller.
	var output strings.Builder
	wrapped := func(s string) {
		output.WriteString(s)
		onChunk(s)
	}
	inputTok, outputTok, llmErr := m.callLLM(ctx, provider, agent, systemPrompt, userMsg, wrapped)

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
		log.Printf("codevaldai: ExecuteRun run=%s agent=%s llm error: %v", runID, agent.ID, llmErr)
		m.publishJSON(ctx, TopicTaskFailed, TaskFailedPayload{
			TaskID: run.TaskID,
			RunID:  runID,
			Reason: errMsg,
		})
		m.publish(ctx, fmt.Sprintf("ai.%s.run.failed", m.agencyID), "")
		failedEntity, _ := m.dm.GetEntity(ctx, m.agencyID, runID)
		failed := agentRunFromEntity(failedEntity)
		failed.AgentID = agent.ID
		return failed, fmt.Errorf("ExecuteRun %s: %w", runID, llmErr)
	}

	finalOutput := output.String()
	log.Printf("codevaldai: ExecuteRun run=%s agent=%s llm ok: input_tokens=%d output_tokens=%d output_len=%d",
		runID, agent.ID, inputTok, outputTok, len(finalOutput))

	updated, err := m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":        string(AgentRunStatusCompleted),
			"output":        finalOutput,
			"input_tokens":  inputTok,
			"output_tokens": outputTok,
			"completed_at":  now,
			"updated_at":    now,
		},
	})
	if err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: store output: %w", runID, err)
	}

	// Dispatch any PubSub actions the LLM embedded in its output.
	m.dispatchActions(ctx, finalOutput)

	m.publishJSON(ctx, TopicTaskCompleted, TaskCompletedPayload{
		TaskID:  run.TaskID,
		RunID:   runID,
		AgentID: agent.ID,
	})
	m.publish(ctx, fmt.Sprintf("ai.%s.run.completed", m.agencyID), `{"run_id":"`+runID+`"}`)

	completed := agentRunFromEntity(updated)
	completed.AgentID = agent.ID
	return completed, nil
}

// dispatchActions parses any ```actions block from the LLM output and
// publishes each action as a PubSub event via CodeValdCross.
func (m *aiManager) dispatchActions(ctx context.Context, output string) {
	if m.publisher == nil {
		return
	}
	actions, err := parseActions(output)
	if err != nil {
		log.Printf("codevaldai: dispatchActions: malformed actions block: %v", err)
		return
	}
	if len(actions) == 0 {
		log.Printf("codevaldai: dispatchActions: no actions block in output")
		return
	}
	log.Printf("codevaldai: dispatchActions: dispatching %d action(s)", len(actions))
	for _, a := range actions {
		log.Printf("codevaldai: dispatchActions: publishing topic=%s", a.Topic)
		if err := m.publisher.Publish(ctx, a.Topic, m.agencyID, "codevaldai", a.RawPayload()); err != nil {
			log.Printf("codevaldai: dispatchActions: publish topic=%s error: %v", a.Topic, err)
		}
	}
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
