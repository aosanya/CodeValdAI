````markdown
# LLM Client — Implementation Details

---

## MVP-AI-006 — LLMClient Interface

**Status**: 🔲 Not Started
**Branch**: `feature/AI-006_llmclient_interface`

### Goal

Define the provider-agnostic `LLMClient` interface and its supporting types in
`internal/llm/client.go`. No provider code here — only the contract.

### File: `internal/llm/client.go`

```go
// Package llm defines the LLMClient interface and its request/response types.
// Concrete implementations live in sub-packages (e.g. anthropic/).
package llm

import "context"

// LLMClient abstracts all LLM provider communication.
// Implementations must be safe for concurrent use.
type LLMClient interface {
    // Complete sends a single completion request to the configured provider
    // and returns the response text and token counts.
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// CompletionRequest is the provider-agnostic input to a single LLM call.
type CompletionRequest struct {
    Model       string  // e.g. "claude-3-5-sonnet-20241022"
    System      string  // System message / persona
    UserMessage string  // Full user-turn content
    Temperature float64 // Sampling temperature (0.0–1.0)
    MaxTokens   int     // Maximum output tokens
}

// CompletionResponse is the provider-agnostic output of a single LLM call.
type CompletionResponse struct {
    Content      string // Raw text output from the LLM
    InputTokens  int    // Tokens consumed by the prompt
    OutputTokens int    // Tokens in the completion
}
```

### Acceptance Tests

- `CompletionRequest` and `CompletionResponse` are zero-value safe
- Interface is satisfied by the Anthropic implementation (compile-time check in `anthropic/client.go`)

---

## MVP-AI-007 — Anthropic Implementation

**Status**: 🔲 Not Started
**Branch**: `feature/AI-007_anthropic_implementation`

### Goal

Implement `LLMClient` for the Anthropic Messages API in `internal/llm/anthropic/client.go`.
Uses the Anthropic REST API (`https://api.anthropic.com/v1/messages`) via `net/http` — no
third-party SDK.

### File: `internal/llm/anthropic/client.go`

```go
// Package anthropic implements llm.LLMClient for the Anthropic Messages API.
package anthropic

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "github.com/aosanya/CodeValdAI/internal/llm"
)

const defaultBaseURL = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"

// Client implements llm.LLMClient using the Anthropic Messages REST API.
type Client struct {
    apiKey  string
    baseURL string
    http    *http.Client
}

// New returns an Anthropic LLMClient.
// apiKey must be non-empty. baseURL defaults to the Anthropic production endpoint.
func New(apiKey, baseURL string) (*Client, error)

// Complete implements llm.LLMClient.Complete.
func (c *Client) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
```

### Request / Response Shapes (Anthropic API)

**Request** (`POST https://api.anthropic.com/v1/messages`):
```json
{
  "model":      "claude-3-5-sonnet-20241022",
  "max_tokens": 4096,
  "system":     "<system_prompt>",
  "messages": [
    { "role": "user", "content": "<user_message>" }
  ]
}
```

**Headers**:
```
x-api-key:         <ANTHROPIC_API_KEY>
anthropic-version: 2023-06-01
content-type:      application/json
```

**Response** (success):
```json
{
  "content": [{ "type": "text", "text": "..." }],
  "usage": { "input_tokens": 42, "output_tokens": 100 }
}
```

### Error Handling

- Non-2xx HTTP status → return `fmt.Errorf("anthropic: HTTP %d: %s", status, body)`
- JSON parse failure → return `fmt.Errorf("anthropic: decode response: %w", err)`
- Context cancellation → propagate `ctx.Err()`

### Configuration

`ANTHROPIC_API_KEY` is read from env in `cmd/main.go` and passed to `anthropic.New`.
The client itself does not read env vars — keeps it testable.

### Acceptance Tests

- `New("", "")` returns an error
- `Complete` with a cancelled context returns `ctx.Err()`
- HTTP 401 response returns a descriptive error (not a panic)
- Successful response maps `content[0].text` to `CompletionResponse.Content`
- Token counts from `usage` are mapped correctly
- Compile-time interface check: `var _ llm.LLMClient = (*Client)(nil)`
````
