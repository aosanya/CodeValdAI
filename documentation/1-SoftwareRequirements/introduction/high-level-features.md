```markdown
# CodeValdAI — High-Level Features

## 1. Agent Catalogue

An **Agent** is a persistent configuration entity:

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | ✅ | Unique identifier |
| `name` | string | ✅ | Human-readable name (e.g. "Risk Analyst") |
| `description` | string | | What this agent does |
| `provider` | string | ✅ | LLM provider — `"anthropic"` \| `"openai"` |
| `model` | string | ✅ | Model identifier (e.g. `"claude-3-5-sonnet-20241022"`) |
| `system_prompt` | string | ✅ | Persona and constraint instructions sent as the system message |
| `temperature` | number | | Sampling temperature (default 0.7) |
| `max_tokens` | number | | Max output tokens (default 4096) |

Multiple agents can exist per Agency, each with a different persona and model binding.

---

## 2. Two-Phase Run Lifecycle

### Phase 1 — Intake

`POST /{agencyID}/ai/runs/intake`

```json
Request:
{
  "agent_id":    "agent-001",
  "workflow_id": "workflow-abc",
  "instructions": "Analyse the attached report and summarise key risks."
}

Response:
{
  "run_id": "run-xyz",
  "fields": [
    { "fieldname": "report_title",  "type": "string",  "label": "Report Title",  "required": true  },
    { "fieldname": "date_range",    "type": "string",  "label": "Date Range",    "required": false },
    { "fieldname": "risk_category", "type": "select",  "label": "Risk Category", "required": true,
      "options": ["financial", "operational", "reputational"] }
  ]
}
```

The `run_id` is created at intake time. The `AgentRun` is stored in `pending_intake` state.

### Phase 2 — Execute

`POST /{agencyID}/ai/runs/{runID}/execute`

```json
Request:
{
  "inputs": [
    { "fieldname": "report_title",  "value": "Q1 2026 Risk Report" },
    { "fieldname": "risk_category", "value": "financial" }
  ]
}

Response:
{
  "run_id":    "run-xyz",
  "status":    "completed",
  "output":    "The Q1 2026 Risk Report identifies three primary financial risks…",
  "input_tokens":  1240,
  "output_tokens": 876,
  "completed_at":  "2026-03-24T10:05:00Z"
}
```

---

## 3. Run Status Lifecycle

```
pending_intake ──► pending_execution ──► running ──► completed
                                                 └──► failed
```

| Status | Meaning |
|---|---|
| `pending_intake` | AgentRun created; waiting for caller to submit filled inputs |
| `pending_execution` | Inputs received; queued for LLM call |
| `running` | LLM call in progress |
| `completed` | LLM returned successfully; output stored |
| `failed` | LLM call failed or timed out; error_message stored |

---

## 4. Provider-Agnostic LLM Client

The `LLMClient` interface abstracts all provider-specific HTTP calls.
`cmd/main.go` constructs the desired implementation and injects it into `AIManager`.

**MVP implementation**: Anthropic (`claude-3-5-sonnet-20241022`)

```go
type LLMClient interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
```

---

## 5. Auto-Dispatch

When CodeValdAI receives a `work.task.dispatched` event from the Cross bus, it
can automatically start a run against the configured agent for that workflow.
Explicit triggering via the HTTP/gRPC API is always available regardless of
auto-dispatch configuration.

---

## 6. Pub/Sub Events

| Topic | Trigger |
|---|---|
| `cross.ai.{agencyID}.run.completed` | `ExecuteRun` completes successfully |
| `cross.ai.{agencyID}.run.failed` | `ExecuteRun` transitions to `failed` |
| `cross.ai.{agencyID}.agent.created` | `CreateAgent` succeeds |
```
