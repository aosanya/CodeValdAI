# Provider — OpenAI-Compatible (OpenAI + HuggingFace Router)

A single dispatcher `callOpenAICompatible` serves two `provider_type` values:

| `provider_type` | Default Base URL | Auth Header | Notes |
|---|---|---|---|
| `openai` | `https://api.openai.com/v1/chat/completions` | `Authorization: Bearer {APIKey}` | Standard OpenAI |
| `huggingface` | `https://router.huggingface.co/v1/chat/completions` | `Authorization: Bearer {APIKey}` | HF Inference Providers / Router |

Both speak OpenAI Chat Completions JSON. The dispatcher only differs in
base URL and (for HuggingFace) the optional `ProviderRoute` suffix on the
model id.

A future direct-Together / direct-Fireworks / local-vLLM provider can be
added with a new `provider_type` enum value and a new `case` in the
dispatch switch in [../dispatcher.md](../dispatcher.md) — no new
dispatcher implementation needed.

---

### Request

```http
POST {LLMProvider.BaseURL or default}
Content-Type: application/json
Authorization: Bearer {LLMProvider.APIKey}
```

```json
{
  "model": "<effective model id>",
  "messages": [
    {"role": "system", "content": "<Agent.SystemPrompt>"},
    {"role": "user",   "content": "<built user message>"}
  ],
  "stream": true,
  "stream_options": {"include_usage": true},
  "temperature": 0.7,
  "max_tokens": 4096
}
```

`stream_options.include_usage` is required to receive token counts on the
final SSE frame. OpenAI honors it; HuggingFace Router honors it on
backends that pass it through (most do — some omit usage anyway, see
token-counting tolerance in [../dispatcher.md](../dispatcher.md)).

### Effective Model ID

```go
effectiveModel := agent.Model
if provider.ProviderType == "huggingface" && provider.ProviderRoute != "" {
    effectiveModel += ":" + provider.ProviderRoute
}
```

For OpenAI, `ProviderRoute` is always ignored.

---

### Response — Streaming SSE

Each frame is an OpenAI-format chat completion chunk:

```
data: {"choices":[{"delta":{"content":"hello"}}]}
data: {"choices":[{"delta":{"content":" world"}}]}
data: {"usage":{"prompt_tokens":42,"completion_tokens":100}}
data: [DONE]
```

The dispatcher calls `onChunk` for each `delta.content` value, accumulates
usage from the frame containing `usage`, and exits on `data: [DONE]`.

If no `usage` frame arrives before `[DONE]`, return `(0, 0, nil)` and log
a warning (see token-counting tolerance in [../dispatcher.md](../dispatcher.md)).

---

### Worked Example — DeepSeek V4 via HuggingFace Router

**Provider configuration (`CreateProviderRequest`):**

```json
{
  "name":           "huggingface-deepseek",
  "provider_type":  "huggingface",
  "api_key":        "hf_xxxxx",
  "provider_route": "fireworks-ai"
}
```

`base_url` left empty → uses `https://router.huggingface.co/v1/chat/completions`.
`provider_route: "fireworks-ai"` pins the Fireworks backend (omit it to
let the router auto-select; pin it for cost/latency/determinism control).

**Agent configuration (`CreateAgentRequest`):**

```json
{
  "name":            "deepseek-reasoner",
  "provider_id":     "<id from above>",
  "model":           "deepseek-ai/DeepSeek-V4",
  "system_prompt":   "You are a careful reasoner...",
  "temperature":     0.3,
  "max_tokens":      8192,
  "timeout_seconds": 600
}
```

`timeout_seconds: 600` overrides the system default (300s) — DeepSeek V4
reasoning runs can take minutes for complex prompts. See
[../dispatcher.md](../dispatcher.md) for the timeout contract.

**Effective request to HuggingFace Router:**

```http
POST https://router.huggingface.co/v1/chat/completions
Authorization: Bearer hf_xxxxx
Content-Type: application/json
```

```json
{
  "model": "deepseek-ai/DeepSeek-V4:fireworks-ai",
  "messages": [
    {"role": "system", "content": "You are a careful reasoner..."},
    {"role": "user",   "content": "<execution user message>"}
  ],
  "stream": true,
  "stream_options": {"include_usage": true},
  "temperature": 0.3,
  "max_tokens": 8192
}
```

**To swap to a different backend without touching the Agent:** create a
second `LLMProvider` with `provider_route: "together"` (or empty for
auto), then update the Agent's `provider_id` via `UpdateAgent`. No change
to `Agent.Model`.

---

### Error Handling

| HTTP status | Mapping |
|---|---|
| 401 | `fmt.Errorf("%s: unauthorized", providerType)` |
| 404 | `fmt.Errorf("%s: model %q not found", providerType, effectiveModel)` (common when `ProviderRoute` doesn't serve `Agent.Model`) |
| 429 | `fmt.Errorf("%s: rate limited", providerType)` |
| 503 | `fmt.Errorf("%s: backend unavailable", providerType)` (HF: backend cold-start or quota) |
| 5xx | `fmt.Errorf("%s: HTTP %d: %s", providerType, status, body)` |
| Mid-stream parse failure | `fmt.Errorf("%s: decode SSE: %w", providerType, err)` |
| `context.DeadlineExceeded` | Propagated |

`providerType` in the message is `"openai"` or `"huggingface"` so logs
disambiguate the two callers of `callOpenAICompatible`.

---

### Acceptance Tests

| Test | Expected |
|---|---|
| OpenAI: successful streaming response | `onChunk` called per delta; usage from final frame |
| HuggingFace: successful streaming with `ProviderRoute` set | Effective model id has `:fireworks-ai` suffix; chunks delivered |
| HuggingFace: successful streaming with `ProviderRoute` empty | Effective model id is bare repo id; chunks delivered |
| OpenAI: `ProviderRoute` set on provider | Field ignored; effective model id is bare |
| HF Router: response with no `usage` frame | `(0, 0, nil)` returned; warning logged; run still completes |
| HTTP 404 with HuggingFace + invalid `ProviderRoute` | Error message references both model and route |
| Cancelled context | `ctx.Err()` returned; partial chunks delivered |
