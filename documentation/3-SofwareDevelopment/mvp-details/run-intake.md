````markdown
# Intake Flow — Implementation Details

---

## MVP-AI-013 — Intake Flow

**Status**: 🔲 Not Started
**Branch**: `feature/AI-013_intake_flow`

### Goal

Implement `AIManager.IntakeRun`. This is Phase 1 of the two-phase run lifecycle.
The method calls the LLM to infer what input fields the agent needs, persists the
`AgentRun` entity in `pending_intake` state, and stores the inferred `RunField` entities.

---

### Full Implementation Walkthrough

```
AIManager.IntakeRun(ctx, IntakeRunRequest{AgentID, WorkflowID, Instructions})

1. Validate inputs:
       agent_id non-empty      → ErrInvalidAgent
       instructions non-empty  → ErrInvalidAgent

2. dm.GetEntity(ctx, agentID) → Agent entity
       not found → ErrAgentNotFound

3. Build the intake system message:
       "You are an AI intake assistant. Given a workflow context and a task
        instruction, return ONLY a JSON array of input fields required to
        complete the task. Each object in the array must have:
          fieldname (string, snake_case)
          type      (one of: string | text | number | boolean | select)
          label     (human-readable label)
          required  (bool)
          options   ([]string, only present when type=select)
        Return nothing else — no prose, no markdown, only the JSON array."

4. Build the intake user message:
       "Workflow ID: {workflowID}
        Instructions: {instructions}
        What input fields do you need to complete this task?"

5. llmClient.Complete(ctx, CompletionRequest{
       Model:       agent.Model,
       System:      <intake system message>,
       UserMessage: <intake user message>,
       Temperature: 0.2,   // low temperature for structured output
       MaxTokens:   512,
   })
       → on LLM error: return nil, nil, fmt.Errorf("IntakeRun %s: llm: %w", agentID, err)

6. Parse the LLM response as []intakeField (internal struct):
       type intakeField struct {
           Fieldname string   `json:"fieldname"`
           Type      string   `json:"type"`
           Label     string   `json:"label"`
           Required  bool     `json:"required"`
           Options   []string `json:"options,omitempty"`
       }
       json.Unmarshal([]byte(response.Content), &fields)
       → ErrInvalidLLMResponse if unmarshal fails or result is empty

7. dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
       TypeID:     "AgentRun",
       AgencyID:   agencyID,
       Properties: map[string]any{
           "agent_id":     agentID,
           "workflow_id":  workflowID,
           "instructions": instructions,
           "status":       "pending_intake",
       },
   }) → AgentRun entity with generated ID

8. For i, f := range fields:
       fieldEntity, _ := dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
           TypeID:   "RunField",
           AgencyID: agencyID,
           Properties: map[string]any{
               "fieldname":  f.Fieldname,
               "type":       f.Type,
               "label":      f.Label,
               "required":   f.Required,
               "options":    marshalOptions(f.Options), // JSON string
               "ordinality": i + 1,
           },
       })
       dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
           FromID: run.ID,
           ToID:   fieldEntity.ID,
           Label:  "has_field",
       })

9. Map internal entities → domain RunField structs (unmarshal options JSON)

10. Return (AgentRun, []RunField, nil)
```

---

### Intake Prompt Design

The intake system message is hardcoded in `ai.go` as a package-level constant.
It must instruct the LLM to return **only** a JSON array — no prose.

```go
const intakeSystemMessage = `You are an AI intake assistant. ...`
```

If the LLM returns prose wrapping the JSON (e.g. "Here are the fields: [...]"),
the parser should attempt to extract the JSON array from within the response
before returning `ErrInvalidLLMResponse`.

---

### RunField.options Serialisation

`RunField.Options` is `[]string` in the domain model but stored as a JSON string
in ArangoDB (`"[\"financial\",\"operational\"]"`). The `intakeManager` handles
marshal/unmarshal at the boundary:

```go
// marshalOptions encodes []string → JSON string for storage.
// Returns "" for nil/empty slices.
func marshalOptions(opts []string) string

// unmarshalOptions decodes JSON string → []string for the domain model.
// Returns nil for empty strings.
func unmarshalOptions(s string) []string
```

---

### Acceptance Tests

| Test | Expected |
|---|---|
| `IntakeRun` with empty `agent_id` | `ErrInvalidAgent` |
| `IntakeRun` with empty `instructions` | `ErrInvalidAgent` |
| `IntakeRun` with unknown `agent_id` | `ErrAgentNotFound` |
| LLM returns unparseable response | `ErrInvalidLLMResponse` |
| LLM returns empty array | `ErrInvalidLLMResponse` |
| Valid request | Returns `AgentRun` with `status="pending_intake"` and non-empty `[]RunField` |
| `AgentRun.ID` is non-empty | ✅ |
| Each `RunField` has `has_field` edge to the `AgentRun` | ✅ |
| Calling `GetRun(runID)` after `IntakeRun` returns the stored run | ✅ |

---

### FakeLLMClient for Tests

```go
// FakeLLMClient returns a hardcoded intake response for testing.
type FakeLLMClient struct {
    Response llm.CompletionResponse
    Err      error
}

func (f *FakeLLMClient) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
    return f.Response, f.Err
}
```

The fake is defined in `internal/llm/fake.go` or `ai_test.go` — never shipped in production binary.
````
