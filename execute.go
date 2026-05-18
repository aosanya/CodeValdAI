package codevaldai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/google/uuid"
)

// SessionLimits holds the resolved per-session execution constraints.
// Zero values mean "no limit" except MaxSessions which is always >= 1.
type SessionLimits struct {
	MaxSeconds  int // 0 = no wall-clock limit
	MaxTokens   int // 0 = no token limit
	MaxSessions int // >= 1; 1 = no yielding
}

// resolveSessionLimits merges agent defaults with work-plan overrides stored
// on the run entity. A non-zero work-plan value takes precedence.
// MaxSessions is clamped to a minimum of 1.
func resolveSessionLimits(agent Agent, wpMaxSecs, wpMaxToks, wpMaxSess int) SessionLimits {
	coalesce := func(vals ...int) int {
		for _, v := range vals {
			if v != 0 {
				return v
			}
		}
		return 0
	}
	limits := SessionLimits{
		MaxSeconds:  coalesce(wpMaxSecs, agent.SessionMaxSeconds, 300),
		MaxTokens:   coalesce(wpMaxToks, agent.SessionMaxTokens),
		MaxSessions: coalesce(wpMaxSess, agent.SessionMaxSessions, 1),
	}
	if limits.MaxSessions < 1 {
		limits.MaxSessions = 1
	}
	return limits
}

// estimateTokens returns a rough token estimate (1 token ≈ 4 characters).
func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 && len(s) > 0 {
		return 1
	}
	return n
}

// ExecuteRun implements AIManager.ExecuteRun.
// Thin wrapper around ExecuteRunStreaming with a no-op onChunk callback.
func (m *aiManager) ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error) {
	return m.ExecuteRunStreaming(ctx, runID, inputs, func(string) {})
}

// ExecuteRunStreaming implements AIManager.ExecuteRunStreaming.
// Phase 2 of the two-phase run lifecycle: validates the run, stores submitted
// inputs (first session only), calls the LLM, then transitions to completed,
// failed, or yielded depending on the outcome.
//
// Yielded sessions: when session limits fire, the run transitions to YIELDED,
// a successor AgentRun is created, and ExecuteRunStreaming is called
// recursively with the chain history replayed in the LLM prompt.
//
// Returns ErrRunNotFound if runID does not exist.
// Returns ErrRunNotIntaked if the run is not in a dispatchable state.
// Returns ErrProviderNotFound if the agent's linked provider does not exist.
func (m *aiManager) ExecuteRunStreaming(ctx context.Context, runID string, inputs []RunInput, onChunk func(string)) (AgentRun, error) {
	runEntity, err := m.dm.GetEntity(ctx, m.agencyID, runID)
	if err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: %w", runID, toRunErr(err))
	}
	run := agentRunFromEntity(runEntity)

	// Normalize segment_number early — used for preamble injection and yield checks.
	segNumber := run.SegmentNumber
	if segNumber < 1 {
		segNumber = 1
	}

	// Accept both pending_intake (first session) and pending_execution (successor).
	if run.Status != AgentRunStatusPendingIntake && run.Status != AgentRunStatusPendingExecution {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: %w", runID, ErrRunNotIntaked)
	}

	// Store inputs and advance to pending_execution only for the first session.
	if run.Status == AgentRunStatusPendingIntake {
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
	}

	agent, provider, err := m.resolveAgentAndProvider(ctx, runID)
	if err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: %w", runID, err)
	}

	// Resolve session limits from agent defaults + work-plan overrides stored on run.
	wpMaxSecs := intProp(runEntity.Properties, "wp_session_max_seconds")
	wpMaxToks := intProp(runEntity.Properties, "wp_session_max_tokens")
	wpMaxSess := intProp(runEntity.Properties, "wp_session_max_sessions")
	limits := resolveSessionLimits(agent, wpMaxSecs, wpMaxToks, wpMaxSess)

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
	systemPrompt := m.buildSystemPrompt(ctx, agent.SystemPrompt, run.Instructions)

	// Inject decomposition awareness for task-driven first-session runs.
	// Todo execution runs (triggered by work.task.todo) have task_id = TodoID, so the
	// preamble is still injected but the LLM skips decomposition per the preamble guard.
	if run.TaskID != "" && segNumber == 1 {
		systemPrompt = buildDecompositionPreamble(run.TaskID, runID, agent.ID) + "\n\n" + systemPrompt
	}

	// Load conversation history for chain sessions (segment 2+).
	var history []ConversationTurn
	if run.ChainID != "" && run.SegmentNumber > 1 {
		priorRuns, err := m.loadChainHistory(ctx, run.ChainID, run.SegmentNumber)
		if err != nil {
			log.Printf("codevaldai: ExecuteRun run=%s: load chain history: %v", runID, err)
		} else {
			history = buildChainConversation(priorRuns, run.Instructions)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":     string(AgentRunStatusRunning),
			"started_at": now,
			"updated_at": now,
		},
	}); err != nil {
		return AgentRun{}, fmt.Errorf("ExecuteRun %s: transition to running: %w", runID, err)
	}

	log.Printf("codevaldai: ExecuteRun run=%s agent=%s segment=%d provider=%s limits={secs:%d toks:%d sess:%d}",
		runID, agent.ID, run.SegmentNumber, provider.Name,
		limits.MaxSeconds, limits.MaxTokens, limits.MaxSessions)
	m.publishJSON(ctx, TopicTaskInProgress, TaskInProgressPayload{
		TaskID:  run.TaskID,
		RunID:   runID,
		AgentID: agent.ID,
	})

	// Build the streaming context: apply session wall-clock limit on top of the
	// agent-level LLM timeout already applied inside callLLM.
	var streamCtx context.Context
	var streamCancel context.CancelFunc
	if limits.MaxSeconds > 0 {
		streamCtx, streamCancel = context.WithTimeout(ctx, time.Duration(limits.MaxSeconds)*time.Second)
	} else {
		streamCtx, streamCancel = context.WithCancel(ctx)
	}
	defer streamCancel()

	// Accumulate output for DB storage while also forwarding chunks to the caller.
	// Token counting drives the token-budget yield path.
	const outputFlushInterval = 50
	var output strings.Builder
	var tokenCount int
	chunksSinceFlush := 0
	log.Printf("codevaldai: stream run=%s agent=%s ── begin ──────────────────────────────", runID, agent.ID)
	wrapped := func(s string) {
		output.WriteString(s)
		onChunk(s)
		fmt.Print(s)
		tokenCount += estimateTokens(s)
		if limits.MaxTokens > 0 && tokenCount >= limits.MaxTokens {
			streamCancel() // triggers context.Canceled on next read
		}
		chunksSinceFlush++
		if chunksSinceFlush >= outputFlushInterval {
			chunksSinceFlush = 0
			m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{ //nolint:errcheck
				Properties: map[string]any{
					"output":     output.String(),
					"updated_at": time.Now().UTC().Format(time.RFC3339),
				},
			})
		}
	}
	inputTok, outputTok, llmErr := m.callLLM(streamCtx, provider, agent, systemPrompt, userMsg, history, wrapped)
	fmt.Println()

	now = time.Now().UTC().Format(time.RFC3339)

	// Determine whether the error is a session-limit signal (yield candidate).
	// Yield is only possible when max_sessions > 1; when max_sessions=1 (the
	// default "no yield" mode) timeouts and cancels fall to the existing failure
	// path so backward compatibility is preserved.
	isYieldSignal := llmErr != nil &&
		limits.MaxSessions > 1 &&
		(errors.Is(llmErr, context.DeadlineExceeded) || errors.Is(llmErr, context.Canceled))

	if isYieldSignal {
		partialOutput := output.String()
		if segNumber < limits.MaxSessions {
			return m.yieldRun(ctx, run, agent.ID, partialOutput, outputTok, onChunk)
		}
		// Max sessions reached — attempt auto-decomposition before failing.
		if run.TaskID != "" {
			if todos := m.autoDecompose(ctx, agent, provider, run, agent.ID, partialOutput); len(todos.Todos) > 0 {
				return m.completeAsDecomposed(ctx, run, agent.ID, partialOutput, todos)
			}
		}
		errMsg := fmt.Sprintf("max sessions (%d) reached without final result at segment %d", limits.MaxSessions, segNumber)
		m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{ //nolint:errcheck
			Properties: map[string]any{
				"status":        string(AgentRunStatusFailed),
				"error_message": errMsg,
				"partial_output": partialOutput,
				"output_tokens": outputTok,
				"completed_at":  now,
				"updated_at":    now,
			},
		})
		log.Printf("codevaldai: ExecuteRun run=%s agent=%s max sessions exhausted", runID, agent.ID)
		m.publishJSON(ctx, TopicTaskFailed, TaskFailedPayload{
			TaskID: run.TaskID,
			RunID:  runID,
			Reason: errMsg,
		})
		m.publish(ctx, TopicRunFailed, "")
		failedEntity, _ := m.dm.GetEntity(ctx, m.agencyID, runID)
		failed := agentRunFromEntity(failedEntity)
		failed.AgentID = agent.ID
		return failed, fmt.Errorf("ExecuteRun %s: %s", runID, errMsg)
	}

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
		m.publish(ctx, TopicRunFailed, "")
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
	m.publish(ctx, TopicRunCompleted, `{"run_id":"`+runID+`"}`)

	completed := agentRunFromEntity(updated)
	completed.AgentID = agent.ID
	return completed, nil
}

// yieldRun marks the current run as YIELDED, publishes ai.task.yielded,
// creates the successor AgentRun (with continues_from edge), and starts
// ExecuteRunStreaming recursively for the successor.
func (m *aiManager) yieldRun(
	ctx context.Context,
	run AgentRun,
	agentID, partialOutput string,
	tokensUsed int,
	onChunk func(string),
) (AgentRun, error) {
	// Assign a chain_id on the first yield (session 1 → 2).
	chainID := run.ChainID
	if chainID == "" {
		chainID = uuid.New().String()
		// Back-fill chain_id + segment_number=1 onto the current (session 1) run.
		m.dm.UpdateEntity(ctx, m.agencyID, run.ID, entitygraph.UpdateEntityRequest{ //nolint:errcheck
			Properties: map[string]any{
				"chain_id":       chainID,
				"segment_number": 1,
			},
		})
		run.SegmentNumber = 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	m.dm.UpdateEntity(ctx, m.agencyID, run.ID, entitygraph.UpdateEntityRequest{ //nolint:errcheck
		Properties: map[string]any{
			"status":         string(AgentRunStatusYielded),
			"partial_output": partialOutput,
			"output_tokens":  tokensUsed,
			"completed_at":   now,
			"updated_at":     now,
		},
	})
	log.Printf("codevaldai: yieldRun run=%s agent=%s chain=%s segment=%d tokens=%d",
		run.ID, agentID, chainID, run.SegmentNumber, tokensUsed)

	m.publishJSON(ctx, TopicTaskYielded, TaskYieldedPayload{
		TaskID:        run.TaskID,
		RunID:         run.ID,
		ChainID:       chainID,
		SegmentNumber: run.SegmentNumber,
		TokensUsed:    tokensUsed,
		PartialOutput: partialOutput,
	})

	// Read WP overrides from the yielded run entity to propagate to the successor.
	runEntity, _ := m.dm.GetEntity(ctx, m.agencyID, run.ID)
	wpMaxSecs := intProp(runEntity.Properties, "wp_session_max_seconds")
	wpMaxToks := intProp(runEntity.Properties, "wp_session_max_tokens")
	wpMaxSess := intProp(runEntity.Properties, "wp_session_max_sessions")

	successorSegment := run.SegmentNumber + 1
	now = time.Now().UTC().Format(time.RFC3339)
	successorEntity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: m.agencyID,
		TypeID:   "AgentRun",
		Properties: map[string]any{
			"task_id":                 run.TaskID,
			"instructions":            run.Instructions,
			"status":                  string(AgentRunStatusPendingExecution),
			"chain_id":                chainID,
			"segment_number":          successorSegment,
			"wp_session_max_seconds":  wpMaxSecs,
			"wp_session_max_tokens":   wpMaxToks,
			"wp_session_max_sessions": wpMaxSess,
			"created_at":              now,
			"updated_at":              now,
		},
	})
	if err != nil {
		return AgentRun{}, fmt.Errorf("yieldRun %s: create successor: %w", run.ID, err)
	}

	// Link successor to its agent.
	if _, err := m.dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
		AgencyID: m.agencyID,
		FromID:   successorEntity.ID,
		ToID:     agentID,
		Name:     "belongs_to_agent",
	}); err != nil {
		return AgentRun{}, fmt.Errorf("yieldRun %s: link successor agent: %w", run.ID, err)
	}

	// continues_from edge: successor → yielded run.
	if _, err := m.dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
		AgencyID: m.agencyID,
		FromID:   successorEntity.ID,
		ToID:     run.ID,
		Name:     "continues_from",
	}); err != nil {
		log.Printf("codevaldai: yieldRun %s: create continues_from edge: %v", run.ID, err)
	}

	log.Printf("codevaldai: yieldRun run=%s created successor=%s segment=%d",
		run.ID, successorEntity.ID, successorSegment)

	return m.ExecuteRunStreaming(ctx, successorEntity.ID, nil, onChunk)
}

// dispatchActions parses any ```actions block from the LLM output and
// publishes each action as a PubSub event via CodeValdCross.
// ai.task.todo events are published to Cross and consumed by CodeValdWork,
// which materialises them as TaskTodo entities and publishes work.task.todo.
func (m *aiManager) dispatchActions(ctx context.Context, output string) {
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
		if m.publisher != nil {
			if err := m.publisher.Publish(ctx, a.Topic, m.agencyID, "codevaldai", a.RawPayload()); err != nil {
				log.Printf("codevaldai: dispatchActions: publish topic=%s error: %v", a.Topic, err)
			}
		}
	}
}

// unmarshalActionPayload round-trips an Action's Payload map through JSON into
// the typed target struct. Returns an error if marshalling or unmarshalling fails.
func unmarshalActionPayload(a Action, target any) error {
	b, err := json.Marshal(a.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return json.Unmarshal(b, target)
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
