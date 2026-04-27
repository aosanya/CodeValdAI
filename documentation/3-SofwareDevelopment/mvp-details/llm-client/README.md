# LLM Client / Dispatch — Implementation Details

Provider-agnostic LLM dispatch for CodeValdAI. The `aiManager` reads
`LLMProvider` entities from the graph at call time and routes via an
internal switch — no injected `LLMClient` interface (deleted in MVP-AI-006).

This folder documents three follow-on tasks that extend the dispatcher to
support a third provider type (HuggingFace, including DeepSeek V4), make
every call streaming-capable internally, and add timeout enforcement plus
crash recovery.

---

## Task Index

| File | Task | Title | Status |
|---|---|---|---|
| [schema.md](schema.md) | MVP-AI-016 | Schema & model updates: `ProviderRoute`, `TimeoutSeconds`, `huggingface` provider type | 🔲 Not Started |
| [dispatcher.md](dispatcher.md) | MVP-AI-017 | Dispatcher refactor: OpenAI-compatible shape, timeout, boot sweep | 🔲 Not Started |
| [streaming.md](streaming.md) | MVP-AI-018 | Streaming RPC: `ExecuteRunStreaming`, dispatcher chunk callback | 🔲 Not Started |

### Provider Reference

| File | Provider Types | Schema |
|---|---|---|
| [providers/anthropic.md](providers/anthropic.md) | `anthropic` | Anthropic Messages |
| [providers/openai-compatible.md](providers/openai-compatible.md) | `openai`, `huggingface` | OpenAI Chat Completions |

---

## Dispatch Architecture (post-MVP-AI-017)

There is no `LLMClient` interface. The `aiManager` implementation reads
the `LLMProvider` entity from the graph at call time and dispatches via
an unexported switch in `ai.go`:

```go
// internal to aiManager — not exported
func (m *aiManager) callLLM(
    ctx context.Context,
    provider LLMProvider,
    agent Agent,
    system, user string,
    onChunk func(string),    // buffer for unary RPCs, forward for streaming
) (inputTok, outputTok int, err error) {
    switch provider.ProviderType {
    case "anthropic":
        return callAnthropic(ctx, provider, agent, system, user, onChunk)
    case "openai", "huggingface":
        return callOpenAICompatible(ctx, provider, agent, system, user, onChunk)
    default:
        return 0, 0, fmt.Errorf("unsupported provider_type %q", provider.ProviderType)
    }
}
```

**Why `openai` and `huggingface` share one dispatcher:** HuggingFace's
Router endpoint (`https://router.huggingface.co/v1/chat/completions`) is
OpenAI Chat Completions–compatible. Same JSON shape, same SSE streaming
format, same `Authorization: Bearer` auth header. Only the base URL and
an optional model-id suffix (see [schema.md](schema.md) §`ProviderRoute`)
differ. Anthropic's Messages API is genuinely different and stays in its
own dispatcher.

---

## Provider Type Enum

| Value | Default Base URL | Wire Schema |
|---|---|---|
| `anthropic` | `https://api.anthropic.com/v1/messages` | Anthropic Messages |
| `openai` | `https://api.openai.com/v1/chat/completions` | OpenAI Chat Completions |
| `huggingface` | `https://router.huggingface.co/v1/chat/completions` | OpenAI Chat Completions |

Adding a new OpenAI-compatible provider (Together direct, Fireworks
direct, local vLLM, etc.) requires only a new enum value and a new `case`
in the dispatch switch — the dispatcher implementation is shared.

---

## Cross-Cutting Decisions

These apply to every provider and are detailed in the per-task files:

- **Always streams internally** (MVP-AI-018): the dispatcher always sends
  `stream: true` to the provider. Two gRPC entrypoints share the same
  dispatcher: `ExecuteRun` buffers and returns once at the end;
  `ExecuteRunStreaming` forwards each chunk to the gRPC client. No
  per-chunk Cross events.
- **Timeout** (MVP-AI-017): every dispatcher call is wrapped in
  `context.WithTimeout`. Default `5 * time.Minute`, overridable per-Agent
  via `Agent.TimeoutSeconds`. On expiry: status → `failed`, publish
  `cross.ai.{agencyID}.run.failed`.
- **Crash recovery** (MVP-AI-017): startup sweep transitions any `running`
  run to `failed` with `error_message = "interrupted by service restart"`
  and publishes `cross.ai.{agencyID}.run.failed`. Same code path as
  timeout — no new FSM state.
- **Token-counting tolerance**: if the provider response omits `usage`
  (some HF Router backends do this on streaming), store `0` for both
  token counts and log a warning. Do not fail the run.

---

## Dependency Chain

```
MVP-AI-016 (schema)
    └── MVP-AI-017 (dispatcher + timeout + recovery)
            └── MVP-AI-018 (streaming RPC)
```

[MVP-AI-012 (Intake)](../run-intake.md) and [MVP-AI-013 (Execute)](../run-execution.md)
— both still 🔲 Not Started in [../../mvp.md](../../mvp.md) — should depend
on MVP-AI-017 so they wire up the new dispatcher signature directly.
MVP-AI-018 can land after 012/013.
