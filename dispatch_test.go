package codevaldai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeProviderServer returns an httptest.Server that emits the given SSE
// frames separated by blank lines, then closes. Each entry should be a full
// "data: ..." line (or empty string for a blank).
func fakeProviderServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			io.WriteString(w, f+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ── callOpenAICompatible ─────────────────────────────────────────────────────

func TestCallOpenAICompatible_StreamsAndUsage(t *testing.T) {
	srv := fakeProviderServer(t, []string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"choices":[{"delta":{"content":" world"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":42,"completion_tokens":7}}`,
		`data: [DONE]`,
	})

	var got strings.Builder
	in, out, err := callOpenAICompatible(
		context.Background(),
		LLMProvider{ProviderType: "openai", APIKey: "k", BaseURL: srv.URL},
		Agent{Model: "gpt-test"},
		"sys", "usr",
		func(s string) { got.WriteString(s) },
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.String() != "hello world" {
		t.Errorf("content: got %q want %q", got.String(), "hello world")
	}
	if in != 42 || out != 7 {
		t.Errorf("tokens: got (%d,%d) want (42,7)", in, out)
	}
}

func TestCallOpenAICompatible_MissingUsageReturnsZero(t *testing.T) {
	srv := fakeProviderServer(t, []string{
		`data: {"choices":[{"delta":{"content":"x"}}]}`,
		`data: [DONE]`,
	})

	in, out, err := callOpenAICompatible(
		context.Background(),
		LLMProvider{ProviderType: "huggingface", APIKey: "k", BaseURL: srv.URL},
		Agent{Model: "deepseek-ai/DeepSeek-V4"},
		"sys", "usr",
		func(string) {},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if in != 0 || out != 0 {
		t.Errorf("expected (0,0) for missing usage, got (%d,%d)", in, out)
	}
}

func TestCallOpenAICompatible_HuggingFaceProviderRouteSuffix(t *testing.T) {
	var seenModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if idx := strings.Index(string(body), `"model":"`); idx >= 0 {
			rest := string(body)[idx+len(`"model":"`):]
			if end := strings.Index(rest, `"`); end >= 0 {
				seenModel = rest[:end]
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	_, _, err := callOpenAICompatible(
		context.Background(),
		LLMProvider{ProviderType: "huggingface", APIKey: "k", BaseURL: srv.URL, ProviderRoute: "fireworks-ai"},
		Agent{Model: "deepseek-ai/DeepSeek-V4"},
		"sys", "usr",
		func(string) {},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "deepseek-ai/DeepSeek-V4:fireworks-ai"
	if seenModel != want {
		t.Errorf("effective model: got %q want %q", seenModel, want)
	}
}

func TestCallOpenAICompatible_OpenAIIgnoresProviderRoute(t *testing.T) {
	var seenModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if idx := strings.Index(string(body), `"model":"`); idx >= 0 {
			rest := string(body)[idx+len(`"model":"`):]
			if end := strings.Index(rest, `"`); end >= 0 {
				seenModel = rest[:end]
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	_, _, err := callOpenAICompatible(
		context.Background(),
		LLMProvider{ProviderType: "openai", APIKey: "k", BaseURL: srv.URL, ProviderRoute: "fireworks-ai"},
		Agent{Model: "gpt-test"},
		"sys", "usr",
		func(string) {},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if seenModel != "gpt-test" {
		t.Errorf("OpenAI must ignore ProviderRoute: got model %q want %q", seenModel, "gpt-test")
	}
}

func TestCallOpenAICompatible_HTTP404ReferencesModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such model", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	_, _, err := callOpenAICompatible(
		context.Background(),
		LLMProvider{ProviderType: "huggingface", APIKey: "k", BaseURL: srv.URL, ProviderRoute: "fireworks-ai"},
		Agent{Model: "deepseek-ai/DeepSeek-V4"},
		"sys", "usr",
		func(string) {},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "deepseek-ai/DeepSeek-V4:fireworks-ai") {
		t.Errorf("error must reference effective model id, got %q", msg)
	}
	if !strings.Contains(msg, "huggingface") {
		t.Errorf("error must reference provider type, got %q", msg)
	}
}

func TestCallOpenAICompatible_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, _, err := callOpenAICompatible(
		context.Background(),
		LLMProvider{ProviderType: "openai", APIKey: "k", BaseURL: srv.URL},
		Agent{Model: "gpt-test"},
		"sys", "usr",
		func(string) {},
	)
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected unauthorized error, got %v", err)
	}
}

// ── callAnthropic ────────────────────────────────────────────────────────────

func TestCallAnthropic_StreamsAndUsage(t *testing.T) {
	srv := fakeProviderServer(t, []string{
		`data: {"type":"message_start","usage":{"input_tokens":10,"output_tokens":0}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"!"}}`,
		`data: {"type":"message_delta","usage":{"input_tokens":10,"output_tokens":2}}`,
	})

	var got strings.Builder
	in, out, err := callAnthropic(
		context.Background(),
		LLMProvider{ProviderType: "anthropic", APIKey: "k", BaseURL: srv.URL},
		Agent{Model: "claude-test"},
		"sys", "usr",
		func(s string) { got.WriteString(s) },
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.String() != "hi!" {
		t.Errorf("content: got %q want %q", got.String(), "hi!")
	}
	if in != 10 || out != 2 {
		t.Errorf("tokens: got (%d,%d) want (10,2)", in, out)
	}
}

func TestCallAnthropic_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	chunks := 0
	_, _, err := callAnthropic(
		context.Background(),
		LLMProvider{ProviderType: "anthropic", APIKey: "k", BaseURL: srv.URL},
		Agent{Model: "claude-test"},
		"sys", "usr",
		func(string) { chunks++ },
	)
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected unauthorized error, got %v", err)
	}
	if chunks != 0 {
		t.Errorf("onChunk must not be called on HTTP error, got %d calls", chunks)
	}
}

// ── callLLM dispatch + timeout ───────────────────────────────────────────────

func TestCallLLM_RoutesByProviderType(t *testing.T) {
	cases := []struct {
		providerType string
		expectRoute  string
	}{
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"huggingface", "openai"}, // OpenAI-compatible endpoint
	}
	for _, tc := range cases {
		t.Run(tc.providerType, func(t *testing.T) {
			var seenAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if v := r.Header.Get("Authorization"); v != "" {
					seenAuth = "openai"
				} else if r.Header.Get("x-api-key") != "" {
					seenAuth = "anthropic"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(srv.Close)

			m := &aiManager{}
			_, _, err := m.callLLM(
				context.Background(),
				LLMProvider{ProviderType: tc.providerType, APIKey: "k", BaseURL: srv.URL},
				Agent{Model: "x"},
				"sys", "usr",
				func(string) {},
			)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if seenAuth != tc.expectRoute {
				t.Errorf("provider_type %q routed via %q, want %q", tc.providerType, seenAuth, tc.expectRoute)
			}
		})
	}
}

func TestCallLLM_UnknownProviderType(t *testing.T) {
	m := &aiManager{}
	_, _, err := m.callLLM(
		context.Background(),
		LLMProvider{ProviderType: "bogus", APIKey: "k"},
		Agent{Model: "x"},
		"sys", "usr",
		func(string) {},
	)
	if err == nil || !strings.Contains(err.Error(), `unsupported provider_type "bogus"`) {
		t.Errorf("expected unsupported provider_type error, got %v", err)
	}
}

func TestCallLLM_PerAgentTimeoutFires(t *testing.T) {
	// Handler waits longer than the per-Agent timeout so the client-side
	// timeout in callLLM is what aborts the request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)

	m := &aiManager{}
	start := time.Now()
	_, _, err := m.callLLM(
		context.Background(),
		LLMProvider{ProviderType: "openai", APIKey: "k", BaseURL: srv.URL},
		Agent{Model: "x", TimeoutSeconds: 1},
		"sys", "usr",
		func(string) {},
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("expected deadline exceeded, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

