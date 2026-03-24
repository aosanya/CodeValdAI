````markdown
# Execute Flow, Auto-Dispatch & Tests — Implementation Details

---

## MVP-AI-014 — Execute Flow

**Status**: 🔲 Not Started
**Branch**: `feature/AI-014_execute_flow`

### Goal

Implement `AIManager.ExecuteRun`. This is Phase 2 of the two-phase run lifecycle.
The method validates that the run is in `pending_intake`, stores the submitted inputs,
calls the LLM with full context, and transitions the run to `completed` or `failed`.

---

### Full Implementation Walkthrough

```
AIManager.ExecuteRun(ctx, runID, []RunInput)

1. dm.GetEntity(ctx, runID) → AgentRun
       not found → ErrRunNotFound

2. Validate run.Status == "pending_intake"
       → ErrRunNotIntaked if any other status

3. For each input in []RunInput:
       dm.CreateEntity(ctx, {TypeID:"RunInput", AgencyID:agencyID,
           Properties: {"fieldname": input.Fieldname, "value": input.Value}})
       dm.CreateRelationship(ctx, {FromID:runID, ToID:inputEntity.ID, Label:"has_input"})

4. Transition: pending_intake → pending_execution
       dm.UpdateEntity(ctx, runID, {"status": "pending_execution"})

5. dm.GetEntity(ctx, run.AgentID) → Agent

6. Load RunFields (to include field labels in prompt):
       dm.TraverseRelationship(ctx, runID, "has_field") → []RunField entities

7. Build execution user message:
       "WorkflowID: {workflowID}
        Instructions: {instructions}
        Input Fields Provided:
          {field.label} ({field.fieldname}): {matchedInput.value}
          ...
        Complete the task."

8. Transition: pending_execution → running; stamp started_at
       dm.UpdateEntity(ctx, runID, {"status":"running", "started_at": now})

9. llmClient.Complete(ctx, CompletionRequest{
       Model:       agent.Model,
       System:      agent.SystemPrompt,
       UserMessage: <execution user message>,
       Temperature: agent.Temperature,
       MaxTokens:   agent.MaxTokens,
   })

10a. On success:
       dm.UpdateEntity(ctx, runID, {
           "status":        "completed",
           "output":        response.Content,
           "input_tokens":  response.InputTokens,
           "output_tokens": response.OutputTokens,
           "completed_at":  now,
           "updated_at":    now,
       })
       publisher.Publish(ctx,
           fmt.Sprintf("cross.ai.%s.run.completed", agencyID), runID)
           // publish errors: log, do not return to caller
       return (AgentRun{status:"completed"}, nil)

10b. On LLM error:
       dm.UpdateEntity(ctx, runID, {
           "status":        "failed",
           "error_message": err.Error(),
           "completed_at":  now,
           "updated_at":    now,
       })
       publisher.Publish(ctx,
           fmt.Sprintf("cross.ai.%s.run.failed", agencyID), runID)
       return (AgentRun{status:"failed"}, err)
```

---

### Execution Prompt — Input Matching

`ExecuteRun` matches each submitted `RunInput` to its corresponding `RunField`
by `fieldname` to include the human-readable label in the prompt:

```
Risk Category (risk_category): financial
Report Title (report_title): Q1 2026 Risk Report
```

Unmatched inputs (no corresponding field) are included with their fieldname only.

---

### Acceptance Tests

| Test | Expected |
|---|---|
| `ExecuteRun` with unknown `runID` | `ErrRunNotFound` |
| `ExecuteRun` on a `completed` run | `ErrRunNotIntaked` |
| `ExecuteRun` on a `running` run | `ErrRunNotIntaked` |
| Valid `ExecuteRun` — LLM succeeds | Run status = `completed`; output non-empty |
| Valid `ExecuteRun` — LLM errors | Run status = `failed`; error_message stored; error returned |
| `completed` run token counts stored | `input_tokens > 0`, `output_tokens > 0` |
| `cross.ai.{id}.run.completed` published | ✅ (fake publisher captures call) |
| `cross.ai.{id}.run.failed` published on failure | ✅ |
| `completed_at` stamped on both success and failure | ✅ |

---

## MVP-AI-015 — Auto-Dispatch Consumer

**Status**: 🔲 Not Started
**Branch**: `feature/AI-015_auto_dispatch`

### Goal

Subscribe to `work.task.dispatched` events from CodeValdCross.
When a matching event arrives, auto-trigger `IntakeRun` + `ExecuteRun`.

### Design

The consumer runs as a background goroutine started in `cmd/main.go`.
It is registered as part of the Cross `Consumes` topic list.

```go
// internal/consumer/consumer.go

// Consumer handles inbound pub/sub events from CodeValdCross.
type Consumer struct {
    manager  codevaldai.AIManager
    agencyID string
    log      *slog.Logger
}

// HandleTaskDispatched is called when a work.task.dispatched event arrives.
// It extracts agent_id, workflow_id, and instructions from the event payload,
// calls IntakeRun, then immediately calls ExecuteRun with no inputs.
// This is the "zero-input" auto-dispatch path — suitable for workflows
// that require no additional caller input.
func (c *Consumer) HandleTaskDispatched(ctx context.Context, payload []byte) error
```

### Event Payload Shape (from CodeValdWork)

```json
{
  "task_id":    "task-001",
  "agency_id":  "agency-abc",
  "agent_id":   "agent-001",
  "workflow_id": "workflow-xyz",
  "instructions": "Process the daily report"
}
```

### Auto-Dispatch vs Explicit Trigger

| Mode | How |
|---|---|
| Explicit | Caller calls `POST /{agencyID}/ai/runs/intake` then `POST /{agencyID}/ai/runs/{runID}/execute` |
| Auto-dispatch | `work.task.dispatched` event → Consumer calls `IntakeRun` → `ExecuteRun` with no inputs |

Auto-dispatch assumes the workflow requires no human-supplied inputs. If the LLM returns
required fields that have no supplied values, the run transitions to `failed` with a
descriptive error message.

### Acceptance Tests

| Test | Expected |
|---|---|
| Valid `task.dispatched` payload → auto-dispatch | Run created; `IntakeRun` + `ExecuteRun` called |
| Malformed payload | Error logged; no run created; no panic |
| Unknown `agent_id` in payload | Error logged; `ErrAgentNotFound`; no panic |
| Cancelled context | Consumer exits cleanly |

---

## MVP-AI-016 — Unit & Integration Tests

**Status**: 🔲 Not Started
**Branch**: `feature/AI-016_tests`

### Goal

Full test coverage for all `AIManager` methods using `fakeDataManager` and
`FakeLLMClient`. Integration tests run against a real ArangoDB instance.

### Test Files

| File | Covers |
|---|---|
| `ai_test.go` | `NewAIManager`, `CreateAgent`, `GetAgent`, `ListAgents`, `DeleteAgent` |
| `intake_test.go` | `IntakeRun` — all happy paths and error paths |
| `execute_test.go` | `ExecuteRun` — all happy paths, error paths, and status transitions |
| `storage/arangodb/storage_test.go` | ArangoDB backend — integration tests with `+build integration` tag |

### Fake DataManager

```go
// fakeDataManager implements entitygraph.DataManager in memory.
// Defined in ai_test.go — not shipped in production binary.
type fakeDataManager struct {
    entities      map[string]entitygraph.Entity
    relationships []entitygraph.Relationship
    mu            sync.RWMutex
}
```

### Coverage Requirements

- Unit tests (fakes): >80% line coverage on `ai.go`
- Integration tests: all 8 gRPC RPCs exercised end-to-end with ArangoDB
- Race detector: `go test -race ./...` must pass

### CI Targets in Makefile

```makefile
test:
    go test -v -race ./...

test-integration:
    go test -v -race -tags integration ./...

coverage:
    go test -race -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out
```
````
