# MVP-AI-016 — Schema & Model Updates

**Status**: 🚀 In Progress
**Branch**: `feature/AI-016_schema_huggingface_timeout`

### Goal

Extend the data model to support HuggingFace as a third provider, allow
backend pinning on the HuggingFace Router (per-provider routing knob), and
allow per-Agent override of the LLM call timeout.

These are pure schema/model additions — no behavior changes. The dispatcher
work that consumes these new fields is [MVP-AI-017](dispatcher.md).

---

### File: `models.go`

#### `LLMProvider` — add `ProviderRoute`

```go
type LLMProvider struct {
    ID            string `json:"id"`
    Name          string `json:"name"`
    ProviderType  string `json:"provider_type"`             // "anthropic" | "openai" | "huggingface"
    APIKey        string `json:"api_key"`
    BaseURL       string `json:"base_url,omitempty"`        // empty = use provider default
    ProviderRoute string `json:"provider_route,omitempty"`  // HuggingFace-only: backend pin
    CreatedAt     string `json:"created_at"`
    UpdatedAt     string `json:"updated_at"`
}
```

`ProviderRoute` is HuggingFace-specific. The dispatcher appends it to
`Agent.Model` when building the request:

| `Agent.Model` | `LLMProvider.ProviderRoute` | Effective model id |
|---|---|---|
| `deepseek-ai/DeepSeek-V4` | `""` | `deepseek-ai/DeepSeek-V4` (router auto-picks backend) |
| `deepseek-ai/DeepSeek-V4` | `"fireworks-ai"` | `deepseek-ai/DeepSeek-V4:fireworks-ai` (pinned) |

For `anthropic` and `openai` providers, `ProviderRoute` is ignored.

The split (model id on `Agent`, backend route on `LLMProvider`) keeps
`Agent.Model` portable: the same Agent definition can be aimed at a
different backend by pointing it at a different `LLMProvider`, with no
edit to the Agent itself.

#### `CreateProviderRequest` and `UpdateProviderRequest`

```go
type CreateProviderRequest struct {
    Name          string `json:"name"`
    ProviderType  string `json:"provider_type"`
    APIKey        string `json:"api_key"`
    BaseURL       string `json:"base_url,omitempty"`
    ProviderRoute string `json:"provider_route,omitempty"`
}

type UpdateProviderRequest struct {
    Name          string `json:"name,omitempty"`
    APIKey        string `json:"api_key,omitempty"`
    BaseURL       string `json:"base_url,omitempty"`
    ProviderRoute string `json:"provider_route,omitempty"`
}
```

`ProviderType` is intentionally **not** in `UpdateProviderRequest` —
changing the provider type of an existing `LLMProvider` invalidates any
`uses_provider` edges that referenced it for a specific request shape.
Callers must delete + recreate.

#### `Agent` — add `TimeoutSeconds`

```go
type Agent struct {
    ID             string  `json:"id"`
    Name           string  `json:"name"`
    Description    string  `json:"description,omitempty"`
    ProviderID     string  `json:"provider_id"`
    Model          string  `json:"model"`
    SystemPrompt   string  `json:"system_prompt"`
    Temperature    float64 `json:"temperature,omitempty"`
    MaxTokens      int     `json:"max_tokens,omitempty"`
    TimeoutSeconds int     `json:"timeout_seconds,omitempty"`  // 0 = system default
    CreatedAt      string  `json:"created_at"`
    UpdatedAt      string  `json:"updated_at"`
}
```

Same field added to `CreateAgentRequest` and `UpdateAgentRequest`.

System default lives in `ai.go`:

```go
const defaultLLMCallTimeout = 5 * time.Minute
```

---

### File: `schema.go`

#### `LLMProvider` TypeDefinition — add `provider_route`

```go
{
    Name: "LLMProvider",
    // ...existing...
    Properties: []types.PropertyDefinition{
        // ...existing...
        {Name: "base_url",       Type: types.PropertyTypeString},
        {Name: "provider_route", Type: types.PropertyTypeString}, // NEW
        {Name: "created_at",     Type: types.PropertyTypeString},
        {Name: "updated_at",     Type: types.PropertyTypeString},
    },
}
```

Update the comment on the `provider_type` property (currently
`"anthropic" (MVP) | "openai" (reserved)` per [schema.go:56](../../../../schema.go#L56)):

```go
// provider_type: "anthropic" | "openai" | "huggingface"
{Name: "provider_type", Type: types.PropertyTypeString, Required: true},
```

#### `Agent` TypeDefinition — add `timeout_seconds`

```go
{
    Name: "Agent",
    // ...existing...
    Properties: []types.PropertyDefinition{
        // ...existing...
        {Name: "max_tokens",      Type: types.PropertyTypeInteger},
        {Name: "timeout_seconds", Type: types.PropertyTypeInteger}, // NEW
        {Name: "created_at",      Type: types.PropertyTypeString},
        {Name: "updated_at",      Type: types.PropertyTypeString},
    },
}
```

Schema seeding remains idempotent — `AISchemaManager.SetSchema` handles
property additions on the existing `ai-schema-v1`.

---

### File: `ai.go`

Provider-type validation in `CreateProvider` accepts the new value:

```go
switch req.ProviderType {
case "anthropic", "openai", "huggingface":
    // ok
default:
    return LLMProvider{}, ErrInvalidProvider
}
```

---

### Acceptance Tests

| Test | Expected |
|---|---|
| `CreateProvider` with `provider_type: "huggingface"` | Persisted; `ProviderRoute` stored if supplied |
| `CreateProvider` with unknown `provider_type` | `ErrInvalidProvider` |
| `UpdateProvider` with new `ProviderRoute` | Updated in place |
| `UpdateProviderRequest` JSON includes `provider_type` | Field ignored (not in struct) |
| `CreateAgent` with `timeout_seconds: 600` | Persisted with `TimeoutSeconds = 600` |
| `CreateAgent` with `timeout_seconds: 0` | Persisted; dispatcher will use system default |
| `DefaultAISchema` re-seed on existing DB | Idempotent; new properties added without data loss |
