package codevaldai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// makeOpenAISSEServer returns an httptest.Server that streams a single OpenAI
// SSE delta chunk containing content, followed by [DONE].
func makeOpenAISSEServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(map[string]any{
			"choices": []any{
				map[string]any{"delta": map[string]any{"content": content}},
			},
		})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
}

// makeOpenAIErrorServer returns an httptest.Server that responds with HTTP 500.
func makeOpenAIErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
}

// makeSlowServer returns an httptest.Server that hangs until the client
// disconnects (or 10 s elapses). Used to trigger context deadline tests.
func makeSlowServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))
}

// ── IntakeRun Tests ───────────────────────────────────────────────────────────

func TestIntakeRun_EmptyAgentID(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      "",
		Instructions: "do something",
	})
	if !errors.Is(err, ErrInvalidAgent) {
		t.Fatalf("got %v, want ErrInvalidAgent", err)
	}
}

func TestIntakeRun_EmptyInstructions(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      "some-id",
		Instructions: "",
	})
	if !errors.Is(err, ErrInvalidAgent) {
		t.Fatalf("got %v, want ErrInvalidAgent", err)
	}
}

func TestIntakeRun_AgentNotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      "no-such",
		Instructions: "do something",
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestIntakeRun_ProviderNotFound(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	// Agent with no uses_provider edge.
	dm.mu.Lock()
	agentID := dm.nextID()
	dm.entities[agentID] = entitygraph.Entity{
		ID: agentID, AgencyID: testAgencyID, TypeID: "Agent",
		Properties: map[string]any{
			"name": "orphan", "model": "gpt-4", "system_prompt": "sp",
		},
	}
	dm.mu.Unlock()

	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "do something",
	})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("got %v, want ErrProviderNotFound", err)
	}
}

func TestIntakeRun_LLMError(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	srv := makeOpenAIErrorServer(t)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "build a dashboard",
	})
	if err == nil {
		t.Fatal("expected error from LLM, got nil")
	}
}

func TestIntakeRun_InvalidJSONResponse(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	srv := makeOpenAISSEServer(t, "this is not json at all")
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "do something",
	})
	if !errors.Is(err, ErrInvalidLLMResponse) {
		t.Fatalf("got %v, want ErrInvalidLLMResponse", err)
	}
}

func TestIntakeRun_EmptyFieldsResponse(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	srv := makeOpenAISSEServer(t, "[]")
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	_, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "do something",
	})
	if !errors.Is(err, ErrInvalidLLMResponse) {
		t.Fatalf("got %v, want ErrInvalidLLMResponse", err)
	}
}

func TestIntakeRun_Success(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	fieldJSON := `[{"fieldname":"target","type":"string","label":"Target System","required":true}]`
	srv := makeOpenAISSEServer(t, fieldJSON)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	run, fields, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "analyse the target system",
	})
	if err != nil {
		t.Fatalf("IntakeRun: %v", err)
	}
	if run.ID == "" {
		t.Fatal("run.ID must be non-empty")
	}
	if run.Status != AgentRunStatusPendingIntake {
		t.Fatalf("expected status %s, got %s", AgentRunStatusPendingIntake, run.Status)
	}
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].Fieldname != "target" || fields[0].Label != "Target System" || !fields[0].Required {
		t.Fatalf("unexpected field: %+v", fields[0])
	}
}

func TestIntakeRun_MultipleFields(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	fieldsJSON := `[
		{"fieldname":"host","type":"string","label":"Host","required":true},
		{"fieldname":"port","type":"number","label":"Port","required":false},
		{"fieldname":"protocol","type":"select","label":"Protocol","required":true,"options":["tcp","udp"]}
	]`
	srv := makeOpenAISSEServer(t, fieldsJSON)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	run, fields, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "scan the host",
	})
	if err != nil {
		t.Fatalf("IntakeRun: %v", err)
	}
	if run.Status != AgentRunStatusPendingIntake {
		t.Fatalf("expected pending_intake, got %s", run.Status)
	}
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[2].Fieldname != "protocol" || len(fields[2].Options) != 2 {
		t.Fatalf("unexpected select field: %+v", fields[2])
	}
}
