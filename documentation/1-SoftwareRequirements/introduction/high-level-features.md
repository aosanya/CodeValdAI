```markdown
# CodeValdAI — High-Level Features

## 1. Agent Catalogue

An **Agent** is a persistent configuration entity, linked to an `LLMProvider`
via a `uses_provider` graph edge:

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | ✅ | Unique identifier |
| `name` | string | ✅ | Human-readable name (e.g. "Risk Analyst") |
| `description` | string | | What this agent does |
| `provider_id` | string | ✅ | FK to an `LLMProvider` entity (resolved from the `uses_provider` edge) |
| `model` | string | ✅ | Model identifier (e.g. `"claude-3-5-sonnet-20241022"`, `"gpt-4o"`, `"deepseek-ai/DeepSeek-V4"`) |
| `system_prompt` | string | ✅ | Persona and constraint instructions sent as the system message |
| `temperature` | number | | Sampling temperature (default 0.7) |
| `max_tokens` | number | | Max output tokens (default 4096) |
| `timeout_seconds` | number | | Per-Agent override of the system LLM-call timeout (default 5 min) |

Multiple agents can exist per Agency, each with a different persona, model
binding, and (via `provider_id`) provider configuration.

### Provider Catalogue

An **LLMProvider** is a separate persistent configuration entity that holds
the connection-level concerns (API key, optional base URL override, optional
HuggingFace backend route). One `LLMProvider` is shared across many `Agent`s
via the `uses_provider` edge:

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | ✅ | Unique identifier |
| `name` | string | ✅ | Human-readable name (e.g. "huggingface-deepseek") |
| `provider_type` | string | ✅ | `"anthropic"` \| `"openai"` \| `"huggingface"` |
| `api_key` | string | ✅ | Provider credential (stored in DB, not env) |
| `base_url` | string | | Override the provider's default endpoint |
| `provider_route` | string | | HuggingFace-only — backend pin (e.g. `"fireworks-ai"` for DeepSeek V4) |

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

## 4. Data-Driven LLM Dispatch

LLM dispatch is **data-driven** — there is no `LLMClient` interface. The
`aiManager` reads the `LLMProvider` entity from the graph at call time and
routes via an internal switch on `LLMProvider.ProviderType`:

| `provider_type` | Wire schema | Default endpoint |
|---|---|---|
| `anthropic` | Anthropic Messages API | `https://api.anthropic.com/v1/messages` |
| `openai` | OpenAI Chat Completions | `https://api.openai.com/v1/chat/completions` |
| `huggingface` | OpenAI Chat Completions (router) | `https://router.huggingface.co/v1/chat/completions` |

The dispatcher always streams from the provider internally; both unary
`ExecuteRun` and streaming `ExecuteRunStreaming` RPCs share the same code
path. Adding a new OpenAI-compatible provider (Together direct, Fireworks
direct, local vLLM, etc.) requires only a new enum value and a new `case`
in the dispatch switch — no new dispatcher implementation.

See [../../3-SofwareDevelopment/mvp-details/llm-client/](../../3-SofwareDevelopment/mvp-details/llm-client/README.md)
for the full dispatcher contract, per-provider request/response shapes, and
the DeepSeek V4 worked example.

---

## 5. Auto-Dispatch (Deferred from MVP)

When CodeValdAI receives a `work.task.dispatched` event from the Cross bus,
it can automatically start a run against the configured agent for that
workflow. Explicit triggering via the HTTP/gRPC API is always available.

> **Status**: Deferred from MVP. The design is documented under Future Work
> in [../../3-SofwareDevelopment/mvp-details/run-execution.md](../../3-SofwareDevelopment/mvp-details/run-execution.md).
> Activate by adding a new task ID to `mvp.md`.

---

## 6. Pub/Sub Events

| Topic | Trigger |
|---|---|
| `cross.ai.{agencyID}.run.completed` | `ExecuteRun` completes successfully |
| `cross.ai.{agencyID}.run.failed` | `ExecuteRun` transitions to `failed` |
| `cross.ai.{agencyID}.agent.created` | `CreateAgent` succeeds |
```
