package codevaldai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultAnthropicURL   = "https://api.anthropic.com/v1/messages"
	defaultOpenAIURL      = "https://api.openai.com/v1/chat/completions"
	defaultHuggingFaceURL = "https://router.huggingface.co/v1/chat/completions"

	anthropicVersion = "2023-06-01"
)

// httpClient is shared across dispatcher calls. No global timeout — the
// per-call context.WithTimeout in callLLM enforces deadlines.
var httpClient = &http.Client{}

// callLLM dispatches a completion request to the configured provider.
// Always streams from the provider internally; onChunk is invoked once per
// streamed delta. Pass a buffering callback for unary RPCs, or a forwarding
// callback for streaming RPCs.
//
// Wraps the call in context.WithTimeout. The timeout is Agent.TimeoutSeconds
// when non-zero, otherwise [defaultLLMCallTimeout]. The returned token counts
// may be 0 if the provider omitted usage info.
func (m *aiManager) callLLM(
	ctx context.Context,
	provider LLMProvider,
	agent Agent,
	system, user string,
	onChunk func(string),
) (inputTok, outputTok int, err error) {
	timeout := defaultLLMCallTimeout
	if agent.TimeoutSeconds > 0 {
		timeout = time.Duration(agent.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch provider.ProviderType {
	case "anthropic":
		return callAnthropic(ctx, provider, agent, system, user, onChunk)
	case "openai", "huggingface":
		return callOpenAICompatible(ctx, provider, agent, system, user, onChunk)
	default:
		return 0, 0, fmt.Errorf("unsupported provider_type %q", provider.ProviderType)
	}
}

// ── Anthropic ────────────────────────────────────────────────────────────────

// callAnthropic streams an Anthropic Messages API completion. ProviderRoute
// is ignored (Anthropic-only). Returns input/output token counts from the
// terminal message_delta event.
func callAnthropic(
	ctx context.Context,
	provider LLMProvider,
	agent Agent,
	system, user string,
	onChunk func(string),
) (inputTok, outputTok int, err error) {
	url := provider.BaseURL
	if url == "" {
		url = defaultAnthropicURL
	}

	body := map[string]any{
		"model":      agent.Model,
		"max_tokens": maxTokensOrDefault(agent.MaxTokens),
		"system":     system,
		"messages":   []any{map[string]any{"role": "user", "content": user}},
		"stream":     true,
	}
	if agent.Temperature != 0 {
		body["temperature"] = agent.Temperature
	}

	resp, err := postJSON(ctx, url, body, map[string]string{
		"x-api-key":         provider.APIKey,
		"anthropic-version": anthropicVersion,
	})
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, anthropicHTTPError(resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "" {
			continue
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return inputTok, outputTok, fmt.Errorf("anthropic: decode stream event: %w", err)
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				onChunk(event.Delta.Text)
			}
		case "message_delta", "message_start":
			if event.Usage.InputTokens > 0 {
				inputTok = event.Usage.InputTokens
			}
			if event.Usage.OutputTokens > 0 {
				outputTok = event.Usage.OutputTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return inputTok, outputTok, context.DeadlineExceeded
		}
		return inputTok, outputTok, fmt.Errorf("anthropic: read stream: %w", err)
	}
	return inputTok, outputTok, nil
}

func anthropicHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("anthropic: unauthorized")
	case http.StatusTooManyRequests:
		return fmt.Errorf("anthropic: rate limited")
	default:
		return fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// ── OpenAI-compatible (OpenAI + HuggingFace) ─────────────────────────────────

// callOpenAICompatible streams a Chat Completions response from an
// OpenAI-compatible provider. Used for both provider_type "openai" and
// "huggingface". For HuggingFace, ProviderRoute (when non-empty) is
// appended to the model id as ":<route>" to pin a backend.
//
// Returns (0, 0, nil) when the provider omits the usage frame — common on
// some HuggingFace Router backends. The run still completes successfully;
// downstream consumers must treat zero token counts as "not reported".
func callOpenAICompatible(
	ctx context.Context,
	provider LLMProvider,
	agent Agent,
	system, user string,
	onChunk func(string),
) (inputTok, outputTok int, err error) {
	url := provider.BaseURL
	if url == "" {
		switch provider.ProviderType {
		case "huggingface":
			url = defaultHuggingFaceURL
		default:
			url = defaultOpenAIURL
		}
	}

	effectiveModel := agent.Model
	if provider.ProviderType == "huggingface" && provider.ProviderRoute != "" {
		effectiveModel += ":" + provider.ProviderRoute
	}

	body := map[string]any{
		"model": effectiveModel,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": user},
		},
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
		"max_tokens":     maxTokensOrDefault(agent.MaxTokens),
	}
	if agent.Temperature != 0 {
		body["temperature"] = agent.Temperature
	}

	resp, err := postJSON(ctx, url, body, map[string]string{
		"Authorization": "Bearer " + provider.APIKey,
	})
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, openAICompatibleHTTPError(resp, provider.ProviderType, effectiveModel)
	}

	usageSeen := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		if payload == "" {
			continue
		}

		var frame struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return inputTok, outputTok, fmt.Errorf("%s: decode SSE: %w", provider.ProviderType, err)
		}
		for _, ch := range frame.Choices {
			if ch.Delta.Content != "" {
				onChunk(ch.Delta.Content)
			}
		}
		if frame.Usage != nil {
			inputTok = frame.Usage.PromptTokens
			outputTok = frame.Usage.CompletionTokens
			usageSeen = true
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return inputTok, outputTok, context.DeadlineExceeded
		}
		return inputTok, outputTok, fmt.Errorf("%s: read stream: %w", provider.ProviderType, err)
	}
	if !usageSeen {
		return 0, 0, nil
	}
	return inputTok, outputTok, nil
}

func openAICompatibleHTTPError(resp *http.Response, providerType, model string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s: unauthorized", providerType)
	case http.StatusNotFound:
		return fmt.Errorf("%s: model %q not found", providerType, model)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%s: rate limited", providerType)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%s: backend unavailable", providerType)
	default:
		return fmt.Errorf("%s: HTTP %d: %s", providerType, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// ── shared helpers ───────────────────────────────────────────────────────────

func postJSON(ctx context.Context, url string, body any, headers map[string]string) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func maxTokensOrDefault(n int) int {
	if n <= 0 {
		return 4096
	}
	return n
}
