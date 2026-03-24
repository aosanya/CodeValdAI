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

`AIManager.IntakeRun(ctx, IntakeRunRequest{AgentID, Instructions})`

```
1. Validate: agent_id and instructions must be non-empty
         → ErrInvalidAgent if either is missing

2. GetAgent(agentID)
         → ErrAgentNotFound if agent does not exist

3. Resolve LLMProvider via uses_provider edge on Agent
         → ErrProviderNotFound if no provider linked

4. Build intake prompt:
   System: agent.SystemPrompt
   User:   "Given the following instructions, return a JSON array of input
            fields you need to complete this task.
            Each field: {fieldname, type, label, required, options?}
            Instructions: {instructions}"

5. callLLM(ctx, provider, agent, system, user)
         → on LLM error: return error to caller (no run entity created)

6. Parse LLM response as []RunField (JSON array)
         → ErrInvalidLLMResponse if unparseable

7. dataManager.CreateEntity — AgentRun{
       instructions, status: "pending_intake",
       created_at, updated_at
   }
   dataManager.CreateRelationship — belongs_to_agent: AgentRun → Agent

8. For each RunField:
       dataManager.CreateEntity — RunField{fieldname, type, label, required, options, ordinality}
       dataManager.CreateRelationship — has_field: AgentRun → RunField

9. Return (AgentRun, []RunField, nil)
```

---

## 3. Execute Flow

`AIManager.ExecuteRun(ctx, runID, []RunInput)`

```
1. GetRun(runID)
         → ErrRunNotFound if run does not exist

2. Validate run.Status == "pending_intake"
         → ErrRunNotIntaked if any other status

3. Resolve Agent via belongs_to_agent edge on AgentRun
4. Resolve LLMProvider via uses_provider edge on Agent
         → ErrProviderNotFound if no provider linked

5. For each RunInput:
       dataManager.CreateEntity — RunInput{fieldname, value}
       dataManager.CreateRelationship — has_input: AgentRun → RunInput

6. Transition: pending_intake → pending_execution
       dataManager.UpdateEntity — AgentRun{status: "pending_execution"}

7. Build execution prompt:
   System: agent.SystemPrompt
   User:   "Instructions: {instructions}
            Inputs:
              {fieldname}: {value}
              ...
            Complete the task."

8. Transition: pending_execution → running
       dataManager.UpdateEntity — AgentRun{status: "running", started_at: now}

9. callLLM(ctx, provider, agent, system, user)

10a. On success:
       dataManager.UpdateEntity — AgentRun{
           status:        "completed",
           output:        response.Content,
           input_tokens:  response.InputTokens,
           output_tokens: response.OutputTokens,
           completed_at:  now,
       }
       publisher.Publish(ctx, "cross.ai.{agencyID}.run.completed", runID)
       return (AgentRun, nil)

10b. On LLM error:
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
1. Validate required fields: name, provider_id, model, system_prompt
         → ErrInvalidAgent if any are missing

2. GetProvider(providerID)
         → ErrProviderNotFound if provider does not exist

3. dataManager.CreateEntity — Agent{
       name, description, model, system_prompt,
       temperature, max_tokens, created_at, updated_at
   }
   dataManager.CreateRelationship — uses_provider: Agent → LLMProvider

4. publisher.Publish(ctx, "cross.ai.{agencyID}.agent.created", agentID)
   (publish errors are logged; never returned to caller)

5. Return (Agent, nil)
```

---

## 5. HTTP Routes

All routes are registered with CodeValdCross on startup and proxied through it.

```
GET    /{agencyId}/ai/providers
POST   /{agencyId}/ai/providers
GET    /{agencyId}/ai/providers/{providerId}
PUT    /{agencyId}/ai/providers/{providerId}
DELETE /{agencyId}/ai/providers/{providerId}

GET    /{agencyId}/ai/agents
POST   /{agencyId}/ai/agents
GET    /{agencyId}/ai/agents/{agentId}
PUT    /{agencyId}/ai/agents/{agentId}
DELETE /{agencyId}/ai/agents/{agentId}

GET    /{agencyId}/ai/agents/{agentId}/runs
POST   /{agencyId}/ai/agents/{agentId}/runs          ← IntakeRun
GET    /{agencyId}/ai/agents/{agentId}/runs/{runId}
POST   /{agencyId}/ai/agents/{agentId}/runs/{runId}/execute  ← ExecuteRun
```

---

## 6. Error Types (`errors.go`)

```go
var (
    // ErrProviderNotFound is returned when a requested provider ID does not exist.
    ErrProviderNotFound = errors.New("llm provider not found")

    // ErrProviderInUse is returned when DeleteProvider is called on a provider
    // that has one or more Agents referencing it.
    ErrProviderInUse = errors.New("llm provider is in use by one or more agents")

    // ErrInvalidProvider is returned when CreateProvider is called with missing
    // required fields or an unsupported provider_type.
    ErrInvalidProvider = errors.New("invalid provider: missing required fields or unsupported type")

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
    // required fields (name, provider_id, model, system_prompt).
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
    case errors.Is(err, ErrProviderNotFound),
         errors.Is(err, ErrAgentNotFound),
         errors.Is(err, ErrRunNotFound):
        return status.Error(codes.NotFound, err.Error())
    case errors.Is(err, ErrInvalidProvider),
         errors.Is(err, ErrInvalidAgent),
         errors.Is(err, ErrInvalidLLMResponse):
        return status.Error(codes.InvalidArgument, err.Error())
    case errors.Is(err, ErrRunNotIntaked),
         errors.Is(err, ErrInvalidRunStatus),
         errors.Is(err, ErrAgentHasActiveRuns),
         errors.Is(err, ErrProviderInUse):
        return status.Error(codes.FailedPrecondition, err.Error())
    default:
        return status.Error(codes.Internal, err.Error())
    }
}
```
