```markdown
# CodeValdAI — Lifecycle, Flows & Errors

> Part of the split architecture. Index: [architecture.md](architecture.md)

---

## 1. AgentRun State Machine

```
pending_intake ──► pending_execution ──► running ──► completed
                                                 └──► failed
```

| Transition | Trigger | Notes |
|---|---|---|
| `→ pending_intake` | `IntakeRun` called | Run entity created; RunFields stored |
| `pending_intake → pending_execution` | `ExecuteRun` called with inputs | RunInputs stored |
| `pending_execution → running` | LLM call begins | `started_at` stamped |
| `running → completed` | LLM returns successfully | Output + token counts stored; `completed_at` stamped; event published |
| `running → failed` | LLM call errors or times out | `error_message` stored; `completed_at` stamped; failure event published |

Invalid transitions return `ErrInvalidRunStatus`.

---

## 2. Intake Flow

`AIManager.IntakeRun(ctx, IntakeRunRequest{AgentID, WorkflowID, Instructions})`

```
1. Validate: agent_id and instructions must be non-empty
         → ErrInvalidAgent if either is missing

2. GetAgent(agentID)
         → ErrAgentNotFound if agent does not exist

3. Build intake prompt:
   System: agent.SystemPrompt
   User:   "Given the following workflow context and instructions,
            return a JSON array of input fields you need to complete this task.
            Each field: {fieldname, type, label, required, options?}
            WorkflowID: {workflowID}
            Instructions: {instructions}"

4. llmClient.Complete(ctx, CompletionRequest{
       Model:       agent.Model,
       System:      agent.SystemPrompt,
       UserMessage: <intake prompt>,
       Temperature: agent.Temperature,
       MaxTokens:   512,  // intake responses are small
   })
         → on LLM error: return error to caller (no run entity created)

5. Parse LLM response as []RunField (JSON array)
         → ErrInvalidLLMResponse if unparseable

6. dataManager.CreateEntity — AgentRun{
       agent_id, workflow_id, instructions,
       status: "pending_intake",
       created_at, updated_at
   }

7. For each RunField:
       dataManager.CreateEntity — RunField{fieldname, type, label, required, options, ordinality}
       dataManager.CreateRelationship — has_field: AgentRun → RunField

8. Return (AgentRun, []RunField, nil)
```

---

## 3. Execute Flow

`AIManager.ExecuteRun(ctx, runID, []RunInput)`

```
1. GetRun(runID)
         → ErrRunNotFound if run does not exist

2. Validate run.Status == "pending_intake"
         → ErrRunNotIntaked if any other status

3. For each RunInput:
       dataManager.CreateEntity — RunInput{fieldname, value}
       dataManager.CreateRelationship — has_input: AgentRun → RunInput

4. Transition run status: pending_intake → pending_execution
       dataManager.UpdateEntity — AgentRun{status: "pending_execution"}

5. GetAgent(run.AgentID)

6. Build execution prompt:
   System: agent.SystemPrompt
   User:   "WorkflowID: {workflowID}
            Instructions: {instructions}
            Inputs:
              {fieldname}: {value}
              ...
            Complete the task."

7. Transition run status: pending_execution → running
       dataManager.UpdateEntity — AgentRun{status: "running", started_at: now}

8. llmClient.Complete(ctx, CompletionRequest{
       Model:       agent.Model,
       System:      agent.SystemPrompt,
       UserMessage: <execution prompt>,
       Temperature: agent.Temperature,
       MaxTokens:   agent.MaxTokens,
   })

9a. On success:
       dataManager.UpdateEntity — AgentRun{
           status:        "completed",
           output:        response.Content,
           input_tokens:  response.InputTokens,
           output_tokens: response.OutputTokens,
           completed_at:  now,
       }
       publisher.Publish(ctx, "cross.ai.{agencyID}.run.completed", runID)
       return (AgentRun, nil)

9b. On LLM error:
       dataManager.UpdateEntity — AgentRun{
           status:        "failed",
           error_message: err.Error(),
           completed_at:  now,
       }
       publisher.Publish(ctx, "cross.ai.{agencyID}.run.failed", runID)
       return (AgentRun, err)
```

---

## 4. CreateAgent Flow

```
1. Validate required fields: name, provider, model, system_prompt
         → ErrInvalidAgent if any are missing

2. dataManager.CreateEntity — Agent{
       name, description, provider, model,
       system_prompt, temperature, max_tokens,
       created_at, updated_at
   }

3. publisher.Publish(ctx, "cross.ai.{agencyID}.agent.created", agentID)
   (publish errors are logged; never returned to caller)

4. Return (Agent, nil)
```

---

## 5. Error Types (`errors.go`)

```go
var (
    // ErrAgentNotFound is returned when a requested agent ID does not exist.
    ErrAgentNotFound = errors.New("agent not found")

    // ErrRunNotFound is returned when a requested run ID does not exist.
    ErrRunNotFound = errors.New("agent run not found")

    // ErrRunNotIntaked is returned when ExecuteRun is called on a run that
    // is not in pending_intake state.
    ErrRunNotIntaked = errors.New("run is not in pending_intake state")

    // ErrInvalidRunStatus is returned when a run status transition is illegal.
    ErrInvalidRunStatus = errors.New("invalid run status transition")

    // ErrInvalidAgent is returned when CreateAgent is called with missing
    // required fields (name, provider, model, system_prompt).
    ErrInvalidAgent = errors.New("invalid agent: missing required fields")

    // ErrAgentHasActiveRuns is returned when DeleteAgent is called on an
    // agent that has runs in pending_intake, pending_execution, or running state.
    ErrAgentHasActiveRuns = errors.New("agent has active runs")

    // ErrInvalidLLMResponse is returned when the LLM response cannot be
    // parsed into the expected structure (e.g. []RunField at Intake).
    ErrInvalidLLMResponse = errors.New("invalid LLM response format")
)
```

### gRPC Status Code Mapping

```go
func toGRPCError(err error) error {
    switch {
    case errors.Is(err, ErrAgentNotFound), errors.Is(err, ErrRunNotFound):
        return status.Error(codes.NotFound, err.Error())
    case errors.Is(err, ErrInvalidAgent), errors.Is(err, ErrInvalidLLMResponse):
        return status.Error(codes.InvalidArgument, err.Error())
    case errors.Is(err, ErrRunNotIntaked), errors.Is(err, ErrInvalidRunStatus),
         errors.Is(err, ErrAgentHasActiveRuns):
        return status.Error(codes.FailedPrecondition, err.Error())
    default:
        return status.Error(codes.Internal, err.Error())
    }
}
```

---

## 6. gRPC Service Definition (outline)

```protobuf
service AIService {
    // Agent catalogue
    rpc CreateAgent(CreateAgentRequest)  returns (Agent);
    rpc GetAgent(GetAgentRequest)        returns (Agent);
    rpc ListAgents(ListAgentsRequest)    returns (ListAgentsResponse);
    rpc DeleteAgent(DeleteAgentRequest)  returns (google.protobuf.Empty);

    // Run lifecycle
    rpc IntakeRun(IntakeRunRequest)       returns (IntakeRunResponse);
    rpc ExecuteRun(ExecuteRunRequest)     returns (AgentRun);
    rpc GetRun(GetRunRequest)             returns (AgentRun);
    rpc ListRuns(ListRunsRequest)         returns (ListRunsResponse);
}
```

---

## 7. HTTP Convenience Routes (proxied via CodeValdCross)

| Method | Path | gRPC Method | Description |
|---|---|---|---|
| `POST` | `/{agencyID}/ai/runs/intake` | `IntakeRun` | Phase 1 — infer input fields |
| `POST` | `/{agencyID}/ai/runs/{runID}/execute` | `ExecuteRun` | Phase 2 — submit inputs and run |
| `GET` | `/{agencyID}/ai/runs` | `ListRuns` | List all runs |
| `GET` | `/{agencyID}/ai/runs/{runID}` | `GetRun` | Get a single run + output |
| `POST` | `/{agencyID}/ai/agents` | `CreateAgent` | Create an Agent |
| `GET` | `/{agencyID}/ai/agents` | `ListAgents` | List all Agents |
| `GET` | `/{agencyID}/ai/agents/{agentID}` | `GetAgent` | Get a single Agent |
| `DELETE` | `/{agencyID}/ai/agents/{agentID}` | `DeleteAgent` | Delete an Agent |
```
