package codevaldai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// decompositionPreamble is prepended to the system prompt for task-driven
// first-session runs. It makes decomposition an internal CodeValdAI concern
// rather than something driven by work-plan instructions.
const decompositionPreamble = `DECOMPOSITION ASSESSMENT — evaluate before acting

Before executing the work-plan instructions below, assess whether this task
involves more than one distinct operation or phase.

Decompose when ANY of the following apply (default to YES for feature work):
  • The task involves implementing code changes — these always have phases:
    branch creation → implementation → commit/PR
  • The task spans multiple independent concerns that can run in parallel
  • There are clearly separable steps (e.g. scaffold → implement → test → review)
  • Completing it end-to-end would require more than one focused action

Do NOT decompose only when the task is a single atomic operation with no
separable steps (e.g. "rename a variable", "update a single config value").

── IF DECOMPOSING ──────────────────────────────────────────────────────────────
Your ENTIRE response must be exactly one actions block — no other text:

` + "```" + `actions
[{"topic":"ai.task.todo","payload":{"parent_task_id":"PARENT_TASK_ID","run_id":"RUN_ID","agent_id":"AGENT_ID","todos":[
  {"title":"Sub-task title","description":"What this sub-task accomplishes","instructions":"<fully self-contained agent prompt>","ordinality":1,"can_run_parallel":true},
  {"title":"Sub-task title","description":"What this sub-task accomplishes","instructions":"<fully self-contained agent prompt>","ordinality":2,"can_run_parallel":false,"depends_on":[1]}
]}}]
` + "```" + `

Sub-task instructions must be fully self-contained and action-oriented:
  • Include the task context (parent_task_id, title, description)
  • Reference only the PubSub action topics listed in ## Available Actions
  • Child runs can ONLY publish PubSub events — they cannot read files,
    run shell commands, or access external systems directly
  • Each instruction must state exactly which topic to emit and what payload
    fields to include (e.g. "emit git.branch.create with repository=X, name=Y")

── IF NOT DECOMPOSING ──────────────────────────────────────────────────────────
Ignore the section above entirely and proceed with your normal execution actions.`

// buildDecompositionPreamble substitutes run-specific IDs into the preamble template.
func buildDecompositionPreamble(taskID, runID, agentID string) string {
	return strings.NewReplacer(
		"PARENT_TASK_ID", taskID,
		"RUN_ID", runID,
		"AGENT_ID", agentID,
	).Replace(decompositionPreamble)
}

// autoDecompose is called when a run exhausts its session budget without
// completing. It makes a focused LLM call to decompose the remaining work
// into sub-tasks. Returns an empty payload if the LLM call fails or produces
// no valid actions block.
func (m *aiManager) autoDecompose(
	ctx context.Context,
	agent Agent,
	provider LLMProvider,
	run AgentRun,
	agentID string,
	partialOutput string,
) TaskTodoPayload {
	log.Printf("codevaldai: autoDecompose run=%s task=%s: session budget exhausted, decomposing remaining work",
		run.ID, run.TaskID)

	const sysMsg = `You are a task recovery planner for a developer AI agent.
A run exhausted its session budget before finishing. Given the original instructions
and any partial output, produce an ai.task.todo actions block covering the REMAINING work.
Each sub-task must have self-contained instructions. Respond with ONLY the actions block.`

	userMsg := fmt.Sprintf(
		"Original instructions:\n%s\n\nWork completed so far (partial):\n%s\n\n"+
			"Decompose the remaining work into sub-tasks. Use parent_task_id=%s and run_id=%s in the payload.",
		run.Instructions, partialOutput, run.TaskID, run.ID,
	)

	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var buf strings.Builder
	_, _, err := m.callLLM(callCtx, provider, agent, sysMsg, userMsg, nil, func(s string) {
		buf.WriteString(s)
	})
	if err != nil {
		log.Printf("codevaldai: autoDecompose run=%s: llm error: %v", run.ID, err)
		return TaskTodoPayload{}
	}

	actions, err := parseActions(buf.String())
	if err != nil || len(actions) == 0 {
		log.Printf("codevaldai: autoDecompose run=%s: no valid actions block in response", run.ID)
		return TaskTodoPayload{}
	}

	for _, a := range actions {
		if a.Topic != TopicTaskTodo {
			continue
		}
		var payload TaskTodoPayload
		if err := unmarshalActionPayload(a, &payload); err != nil {
			log.Printf("codevaldai: autoDecompose run=%s: unmarshal: %v", run.ID, err)
			continue
		}
		if payload.ParentTaskID == "" {
			payload.ParentTaskID = run.TaskID
		}
		if payload.RunID == "" {
			payload.RunID = run.ID
		}
		if payload.AgentID == "" {
			payload.AgentID = agentID
		}
		return payload
	}
	return TaskTodoPayload{}
}

// completeAsDecomposed stores the decomposition output on the run, publishes
// ai.task.todo, spawns child runs, and transitions the run to completed.
// decomposedOutput is the raw LLM output (the actions block text) to store.
func (m *aiManager) completeAsDecomposed(
	ctx context.Context,
	run AgentRun,
	agentID string,
	decomposedOutput string,
	todos TaskTodoPayload,
) (AgentRun, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	updated, err := m.dm.UpdateEntity(ctx, m.agencyID, run.ID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":       string(AgentRunStatusCompleted),
			"output":       decomposedOutput,
			"completed_at": now,
			"updated_at":   now,
		},
	})
	if err != nil {
		return AgentRun{}, fmt.Errorf("completeAsDecomposed %s: %w", run.ID, err)
	}

	m.publishJSON(ctx, TopicTaskTodo, todos)
	m.publishJSON(ctx, TopicTaskCompleted, TaskCompletedPayload{
		TaskID:  run.TaskID,
		RunID:   run.ID,
		AgentID: agentID,
	})
	m.publish(ctx, TopicRunCompleted, `{"run_id":"`+run.ID+`"}`)

	log.Printf("codevaldai: completeAsDecomposed run=%s: ai.task.todo published with %d item(s)", run.ID, len(todos.Todos))
	completed := agentRunFromEntity(updated)
	completed.AgentID = agentID
	return completed, nil
}
