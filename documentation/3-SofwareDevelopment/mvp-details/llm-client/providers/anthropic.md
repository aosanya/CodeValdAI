# Provider — Anthropic Messages API

`provider_type = "anthropic"`. Default base URL: `https://api.anthropic.com/v1/messages`.

The Anthropic Messages API has a different request/response shape than
OpenAI Chat Completions, so it has its own dispatcher (`callAnthropic`).
HuggingFace and OpenAI share `callOpenAICompatible` —
see [openai-compatible.md](openai-compatible.md).

---

### Request

```http
POST {LLMProvider.BaseURL or default}
Content-Type: application/json
x-api-key: {LLMProvider.APIKey}
anthropic-version: 2023-06-01
```

```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 4096,
  "system": "<Agent.SystemPrompt>",
  "messages": [
    {"role": "user", "content": "<built user message>"}
  ],
  "stream": true,
  "temperature": 0.7
}
```

The dispatcher always sends `stream: true` (see [../streaming.md](../streaming.md)).
`temperature` is omitted when `Agent.Temperature == 0`.
`LLMProvider.ProviderRoute` is **ignored** for Anthropic.

---

### Response — Streaming Event Stream

Anthropic streams Server-Sent Events with one of these event types per
frame: `message_start`, `content_block_start`, `content_block_delta`,
`content_block_stop`, `message_delta`, `message_stop`.

The dispatcher consumes the stream and calls `onChunk` for each
`content_block_delta` event:

```json
{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}
```

Token counts arrive on the final `message_delta` event:

```json
{"type":"message_delta","usage":{"input_tokens":42,"output_tokens":100}}
```

Anthropic always returns `usage` — the token-counting tolerance branch in
[../dispatcher.md](../dispatcher.md) is exercised only by HuggingFace
Router backends.

---

### Error Handling

| HTTP status | Mapping |
|---|---|
| 401 | `fmt.Errorf("anthropic: unauthorized")` |
| 429 | `fmt.Errorf("anthropic: rate limited")` |
| 5xx | `fmt.Errorf("anthropic: HTTP %d: %s", status, body)` |
| Mid-stream parse failure | `fmt.Errorf("anthropic: decode stream event: %w", err)` |
| `context.DeadlineExceeded` | Propagated (timeout path in [../dispatcher.md](../dispatcher.md)) |

There are no Anthropic-specific sentinel errors. `ErrInvalidLLMResponse`
is reserved for cases where the LLM returns content that fails downstream
parsing (e.g. malformed field schema during Intake), not for transport
errors.

---

### Acceptance Tests

| Test | Expected |
|---|---|
| Successful streaming response | `onChunk` called per `content_block_delta`; final usage tokens returned |
| HTTP 401 | Error returned; `onChunk` not called |
| HTTP 429 | Error returned; `onChunk` not called |
| Stream truncated mid-response | Error returned; partial chunks delivered before failure |
| Cancelled context | `ctx.Err()` returned; partial chunks delivered |
