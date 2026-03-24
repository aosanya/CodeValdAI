// Package llm provides the provider-agnostic LLM client interface used by
// [codevaldai.AIManager]. Concrete implementations live in sub-packages
// (e.g. anthropic/).
package llm

import "context"

// LLMClient abstracts LLM provider communication.
// Implementations must be safe for concurrent use.
type LLMClient interface {
	// Complete sends a completion request to the configured LLM provider
	// and returns the response. The caller is responsible for constructing
	// the full prompt (system message + user message) in CompletionRequest.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// CompletionRequest is the provider-agnostic input to a single LLM call.
type CompletionRequest struct {
	// Model is the provider-specific model identifier,
	// e.g. "claude-3-5-sonnet-20241022".
	Model string

	// System is the system message / persona instructions sent to the model.
	System string

	// UserMessage is the full user-turn content for this request.
	UserMessage string

	// Temperature controls sampling randomness (0.0–1.0).
	Temperature float64

	// MaxTokens is the maximum number of tokens to generate in the response.
	MaxTokens int
}

// CompletionResponse is the provider-agnostic output of a single LLM call.
type CompletionResponse struct {
	// Content is the raw text output returned by the LLM.
	Content string

	// InputTokens is the number of tokens consumed by the prompt.
	InputTokens int

	// OutputTokens is the number of tokens in the completion.
	OutputTokens int
}
