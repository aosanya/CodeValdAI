package codevaldai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ── ExecuteRun Tests ──────────────────────────────────────────────────────────

func TestExecuteRun_RunNotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.ExecuteRun(context.Background(), "no-such", nil)
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("got %v, want ErrRunNotFound", err)
	}
}

func TestExecuteRun_NotInPendingIntake(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	// Insert a run already in "running" state — not eligible for execute.
	dm.mu.Lock()
	runID := dm.nextID()
	dm.entities[runID] = entitygraph.Entity{
		ID: runID, AgencyID: testAgencyID, TypeID: "AgentRun",
		Properties: map[string]any{"status": string(AgentRunStatusRunning)},
	}
	dm.mu.Unlock()

	_, err := mgr.ExecuteRun(context.Background(), runID, nil)
	if !errors.Is(err, ErrRunNotIntaked) {
		t.Fatalf("got %v, want ErrRunNotIntaked", err)
	}
}

func TestExecuteRun_Success(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)

	srv := makeOpenAISSEServer(t, "Hello from the model!")
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)
	runID := seedRunInPendingIntake(t, dm, agentID)

	run, err := mgr.ExecuteRun(context.Background(), runID, []RunInput{
		{Fieldname: "target", Value: "prod-server"},
	})
	if err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	if run.Status != AgentRunStatusCompleted {
		t.Fatalf("expected status %s, got %s", AgentRunStatusCompleted, run.Status)
	}
	if !strings.Contains(run.Output, "Hello from the model!") {
		t.Fatalf("output must contain LLM response, got: %q", run.Output)
	}

	want := fmt.Sprintf("cross.ai.%s.run.completed", testAgencyID)
	topics := pub.published()
	if len(topics) == 0 || topics[len(topics)-1] != want {
		t.Fatalf("expected topic %q, got %v", want, topics)
	}
}

func TestExecuteRun_LLMError_PublishesFailedEvent(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)

	srv := makeOpenAIErrorServer(t)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)
	runID := seedRunInPendingIntake(t, dm, agentID)

	_, err := mgr.ExecuteRun(context.Background(), runID, nil)
	if err == nil {
		t.Fatal("expected error from LLM, got nil")
	}

	storedRun, getErr := mgr.GetRun(context.Background(), runID)
	if getErr != nil {
		t.Fatalf("GetRun after failure: %v", getErr)
	}
	if storedRun.Status != AgentRunStatusFailed {
		t.Fatalf("expected status %s after LLM error, got %s", AgentRunStatusFailed, storedRun.Status)
	}
	if storedRun.ErrorMessage == "" {
		t.Fatal("ErrorMessage must be non-empty on failure")
	}

	want := fmt.Sprintf("cross.ai.%s.run.failed", testAgencyID)
	topics := pub.published()
	if len(topics) == 0 || topics[len(topics)-1] != want {
		t.Fatalf("expected topic %q, got %v", want, topics)
	}
}

func TestExecuteRun_Timeout(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)

	srv := makeSlowServer(t)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)
	runID := seedRunInPendingIntake(t, dm, agentID)

	// 50 ms deadline — much shorter than the server's 10 s sleep.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := mgr.ExecuteRun(ctx, runID, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	storedRun, getErr := mgr.GetRun(context.Background(), runID)
	if getErr != nil {
		t.Fatalf("GetRun after timeout: %v", getErr)
	}
	if storedRun.Status != AgentRunStatusFailed {
		t.Fatalf("expected status %s after timeout, got %s", AgentRunStatusFailed, storedRun.Status)
	}
	if !strings.Contains(storedRun.ErrorMessage, "timeout exceeded after") {
		t.Fatalf("expected timeout message, got %q", storedRun.ErrorMessage)
	}

	want := fmt.Sprintf("cross.ai.%s.run.failed", testAgencyID)
	topics := pub.published()
	if len(topics) == 0 || topics[len(topics)-1] != want {
		t.Fatalf("expected topic %q, got %v", want, topics)
	}
}

func TestExecuteRunStreaming_ChunksDelivered(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	srv := makeOpenAISSEServer(t, "streamed content here")
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)
	runID := seedRunInPendingIntake(t, dm, agentID)

	var chunks []string
	_, err := mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("ExecuteRunStreaming: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk, got none")
	}
	combined := strings.Join(chunks, "")
	if !strings.Contains(combined, "streamed content here") {
		t.Fatalf("chunks must contain LLM response, got: %q", combined)
	}
}

func TestExecuteRunStreaming_SameOutputAsUnary(t *testing.T) {
	const content = "deterministic output value"

	buildRun := func(baseURL string) (AgentRun, error) {
		dm := newFakeDM()
		mgr, _ := newTestManager(dm)
		_, agentID := seedAgentWithProvider(t, dm, baseURL)
		runID := seedRunInPendingIntake(t, dm, agentID)
		return mgr.ExecuteRun(context.Background(), runID, nil)
	}

	srv1 := makeOpenAISSEServer(t, content)
	defer srv1.Close()
	unaryRun, err := buildRun(srv1.URL)
	if err != nil {
		t.Fatalf("ExecuteRun (unary): %v", err)
	}

	srv2 := makeOpenAISSEServer(t, content)
	defer srv2.Close()
	dm2 := newFakeDM()
	mgr2, _ := newTestManager(dm2)
	_, agentID2 := seedAgentWithProvider(t, dm2, srv2.URL)
	runID2 := seedRunInPendingIntake(t, dm2, agentID2)
	streamingRun, err := mgr2.ExecuteRunStreaming(context.Background(), runID2, nil, func(string) {})
	if err != nil {
		t.Fatalf("ExecuteRunStreaming: %v", err)
	}

	if unaryRun.Output != streamingRun.Output {
		t.Fatalf("output mismatch: unary=%q streaming=%q", unaryRun.Output, streamingRun.Output)
	}
}

// TestIntakeToExecute_EndToEnd runs both phases back-to-back on the same run
// and verifies that field labels from intake flow into the execute prompt
// via the has_field edge.
func TestIntakeToExecute_EndToEnd(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)

	fieldJSON := `[{"fieldname":"host","type":"string","label":"Target Host","required":true}]`
	intakeSrv := makeOpenAISSEServer(t, fieldJSON)
	defer intakeSrv.Close()
	_, agentID := seedAgentWithProvider(t, dm, intakeSrv.URL)

	run, fields, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "scan the host",
	})
	if err != nil {
		t.Fatalf("IntakeRun: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("expected 1 field from intake, got %d", len(fields))
	}

	// Switch the provider URL to the execute server before executing.
	executeSrv := makeOpenAISSEServer(t, "scan complete")
	defer executeSrv.Close()
	dm.mu.Lock()
	// Update provider base_url to the execute server.
	for id, e := range dm.entities {
		if e.TypeID == "LLMProvider" {
			e.Properties["base_url"] = executeSrv.URL
			dm.entities[id] = e
		}
	}
	dm.mu.Unlock()

	completed, err := mgr.ExecuteRun(context.Background(), run.ID, []RunInput{
		{Fieldname: "host", Value: "192.168.1.1"},
	})
	if err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	if completed.Status != AgentRunStatusCompleted {
		t.Fatalf("expected completed, got %s", completed.Status)
	}
	if !strings.Contains(completed.Output, "scan complete") {
		t.Fatalf("unexpected output: %q", completed.Output)
	}

	topics := pub.published()
	wantCompleted := fmt.Sprintf("cross.ai.%s.run.completed", testAgencyID)
	if len(topics) == 0 || topics[len(topics)-1] != wantCompleted {
		t.Fatalf("expected run.completed event, got %v", topics)
	}
}
