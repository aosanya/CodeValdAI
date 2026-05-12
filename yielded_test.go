package codevaldai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeHangingStreamServer returns an httptest.Server that sends the 200 SSE
// header immediately, then keeps the connection open until the client
// disconnects (or 30 s elapse). Unlike makeSlowServer it does not close the
// response body, so the scanner stays blocked until the context cancels.
func makeHangingStreamServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	}))
}

// seedAgentWithSessionLimits inserts an Agent+Provider with the given session
// limits and returns (providerID, agentID).
func seedAgentWithSessionLimits(t *testing.T, dm *fakeDataManager, srvURL string,
	maxSeconds, maxTokens, maxSessions int,
) (string, string) {
	t.Helper()
	dm.mu.Lock()
	defer dm.mu.Unlock()

	providerID := dm.nextID()
	dm.entities[providerID] = entitygraph.Entity{
		ID:       providerID,
		AgencyID: testAgencyID,
		TypeID:   "LLMProvider",
		Properties: map[string]any{
			"name":          "Test Provider",
			"provider_type": "openai",
			"api_key":       "test-key",
			"base_url":      srvURL,
		},
	}

	agentID := dm.nextID()
	dm.entities[agentID] = entitygraph.Entity{
		ID:       agentID,
		AgencyID: testAgencyID,
		TypeID:   "Agent",
		Properties: map[string]any{
			"name":                 "Test Agent",
			"model":                "gpt-4",
			"system_prompt":        "you are a test agent",
			"session_max_seconds":  maxSeconds,
			"session_max_tokens":   maxTokens,
			"session_max_sessions": maxSessions,
		},
	}

	relID := dm.nextID()
	dm.relationships[relID] = entitygraph.Relationship{
		ID:       relID,
		AgencyID: testAgencyID,
		Name:     "uses_provider",
		FromID:   agentID,
		ToID:     providerID,
	}
	return providerID, agentID
}

// seedRunWithWPOverrides inserts an AgentRun in pending_intake with given
// work-plan session overrides.
func seedRunWithWPOverrides(t *testing.T, dm *fakeDataManager, agentID string,
	wpMaxSecs, wpMaxToks, wpMaxSess int,
) string {
	t.Helper()
	dm.mu.Lock()
	defer dm.mu.Unlock()

	runID := dm.nextID()
	dm.entities[runID] = entitygraph.Entity{
		ID:       runID,
		AgencyID: testAgencyID,
		TypeID:   "AgentRun",
		Properties: map[string]any{
			"instructions":            "test instructions",
			"status":                  string(AgentRunStatusPendingIntake),
			"segment_number":          1,
			"wp_session_max_seconds":  wpMaxSecs,
			"wp_session_max_tokens":   wpMaxToks,
			"wp_session_max_sessions": wpMaxSess,
		},
	}
	relID := dm.nextID()
	dm.relationships[relID] = entitygraph.Relationship{
		ID:       relID,
		AgencyID: testAgencyID,
		Name:     "belongs_to_agent",
		FromID:   runID,
		ToID:     agentID,
	}
	return runID
}

// makeStreamingPauseServer returns a server that sends `firstChunk` content,
// flushes, then blocks until the client disconnects (or 5 s elapse).
// Used to test token-limit yields: client cancels after seeing enough tokens.
func makeStreamingPauseServer(t *testing.T, firstChunk string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"delta": map[string]any{"content": firstChunk}}},
		})
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
		// Block until client disconnects — allows context cancel to propagate.
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
}

// findRelationshipByName returns the first relationship with the given name and
// fromID in the fakeDataManager, or nil if none is found.
func findRelationshipByName(dm *fakeDataManager, fromID, name string) *entitygraph.Relationship {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	for _, r := range dm.relationships {
		if r.FromID == fromID && r.Name == name {
			cp := r
			return &cp
		}
	}
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestYieldedSession_WallClockLimit verifies that a session hitting its
// wall-clock limit transitions to YIELDED and publishes ai.task.yielded.
func TestYieldedSession_WallClockLimit(t *testing.T) {
	dm := newFakeDM()
	srv := makeHangingStreamServer(t) // keeps SSE connection open
	defer srv.Close()

	// Agent allows 2 sessions, 1-second wall-clock limit.
	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 1, 0, 2)
	// WP overrides: use agent defaults (zeros).
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, pub := newTestManager(dm)
	_, err := mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {})
	// The successor session also hits the wall-clock limit and exhausts max_sessions → fails.
	// We only verify the first run's state here.

	// Regardless of overall error, the first run must be YIELDED.
	firstRun, getErr := mgr.GetRun(context.Background(), runID)
	if getErr != nil {
		t.Fatalf("GetRun: %v", getErr)
	}
	if firstRun.Status != AgentRunStatusYielded {
		t.Fatalf("expected status yielded, got %s (ExecuteRun err: %v)", firstRun.Status, err)
	}
	if firstRun.ChainID == "" {
		t.Error("chain_id must be set on yielded run")
	}
	if firstRun.SegmentNumber != 1 {
		t.Errorf("segment_number: got %d, want 1", firstRun.SegmentNumber)
	}

	// ai.task.yielded must have been published.
	topics := pub.published()
	if !containsTopic(topics, TopicTaskYielded) {
		t.Errorf("ai.task.yielded not published; got topics: %v", topics)
	}
}

// TestYieldedSession_TokenLimit verifies that a session hitting its token
// budget transitions to YIELDED.
func TestYieldedSession_TokenLimit(t *testing.T) {
	dm := newFakeDM()

	// First chunk is 80 chars → estimateTokens = 20 tokens; limit = 5 → yield.
	firstChunk := strings.Repeat("abcdefghij", 8) // 80 chars
	srv := makeStreamingPauseServer(t, firstChunk)
	defer srv.Close()

	// Agent: 2 sessions, token limit = 5, no wall-clock limit.
	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 0, 5, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, pub := newTestManager(dm)
	mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {}) //nolint:errcheck

	firstRun, err := mgr.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if firstRun.Status != AgentRunStatusYielded {
		t.Errorf("expected yielded, got %s", firstRun.Status)
	}
	if !containsTopic(pub.published(), TopicTaskYielded) {
		t.Errorf("ai.task.yielded not published; topics: %v", pub.published())
	}
}

// TestYieldedSession_MaxSessionsExhausted verifies that when a session yields
// and there are no more sessions available, ai.task.failed is published and
// no ai.task.yielded is emitted for that final session.
func TestYieldedSession_MaxSessionsExhausted(t *testing.T) {
	dm := newFakeDM()
	srv := makeStreamingPauseServer(t, strings.Repeat("x", 80)) // 20 tokens
	defer srv.Close()

	// 2 sessions, token limit = 5 → session 1 yields → session 2 hits limit and max_sessions = 2.
	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 0, 5, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, pub := newTestManager(dm)
	mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {}) //nolint:errcheck

	topics := pub.published()

	// ai.task.yielded from session 1.
	if !containsTopic(topics, TopicTaskYielded) {
		t.Errorf("ai.task.yielded from session 1 not published; topics: %v", topics)
	}
	// ai.task.failed from session 2 exhaustion.
	if !containsTopic(topics, TopicTaskFailed) {
		t.Errorf("ai.task.failed not published; topics: %v", topics)
	}
	// ai.task.yielded must NOT appear as the last task event (ai.task.failed is final).
	lastTaskEvent := lastTopicMatching(topics, TopicTaskYielded, TopicTaskFailed)
	if lastTaskEvent != TopicTaskFailed {
		t.Errorf("expected ai.task.failed as last task event, got %q; topics: %v", lastTaskEvent, topics)
	}
}

// TestYieldedSession_SuccessorHasContinuesFromEdge verifies the continues_from
// edge is created between the successor and the yielded run.
func TestYieldedSession_SuccessorHasContinuesFromEdge(t *testing.T) {
	dm := newFakeDM()
	srv := makeStreamingPauseServer(t, strings.Repeat("x", 80))
	defer srv.Close()

	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 0, 5, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, _ := newTestManager(dm)
	mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {}) //nolint:errcheck

	// Find the successor run (segment 2).
	var successorID string
	dm.mu.RLock()
	for _, e := range dm.entities {
		if e.TypeID == "AgentRun" && e.ID != runID {
			if sn, ok := e.Properties["segment_number"]; ok {
				if intVal(sn) == 2 {
					successorID = e.ID
				}
			}
		}
	}
	dm.mu.RUnlock()

	if successorID == "" {
		t.Fatal("successor run (segment 2) not found")
	}

	rel := findRelationshipByName(dm, successorID, "continues_from")
	if rel == nil {
		t.Fatal("continues_from edge not found from successor to predecessor")
	}
	if rel.ToID != runID {
		t.Errorf("continues_from.ToID = %q, want %q", rel.ToID, runID)
	}
}

// TestYieldedSession_ChainIDSharedAcrossChain verifies that chain_id is the
// same on all runs in a chain.
func TestYieldedSession_ChainIDSharedAcrossChain(t *testing.T) {
	dm := newFakeDM()
	srv := makeStreamingPauseServer(t, strings.Repeat("x", 80))
	defer srv.Close()

	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 0, 5, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, _ := newTestManager(dm)
	mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {}) //nolint:errcheck

	firstRun, _ := mgr.GetRun(context.Background(), runID)
	if firstRun.ChainID == "" {
		t.Fatal("chain_id not set on first run")
	}

	// Find successor and verify chain_id matches.
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	for _, e := range dm.entities {
		if e.TypeID == "AgentRun" && e.ID != runID {
			chainID, _ := e.Properties["chain_id"].(string)
			if chainID != firstRun.ChainID {
				t.Errorf("successor run %q has chain_id %q, want %q", e.ID, chainID, firstRun.ChainID)
			}
		}
	}
}

// TestYieldedSession_TwoSessionChainCompletes verifies a two-session chain
// where session 1 yields and session 2 completes successfully.
func TestYieldedSession_TwoSessionChainCompletes(t *testing.T) {
	dm := newFakeDM()

	var calls atomic.Int32
	// Session 1: return a large chunk then hang (triggers token yield).
	// Session 2: return valid content then [DONE] (completes).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := int(calls.Add(1))
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if call == 1 {
			// Session 1: large chunk then hang.
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": strings.Repeat("x", 80)}}},
			})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		} else {
			// Session 2: complete normally.
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": "final answer"}}},
			})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		}
	}))
	defer srv.Close()

	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 0, 5, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, pub := newTestManager(dm)
	_, err := mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {})
	if err != nil {
		t.Fatalf("expected success on session 2, got error: %v", err)
	}

	topics := pub.published()
	if !containsTopic(topics, TopicTaskYielded) {
		t.Errorf("ai.task.yielded not published; topics: %v", topics)
	}
	if !containsTopic(topics, TopicTaskCompleted) {
		t.Errorf("ai.task.completed not published; topics: %v", topics)
	}
	if !containsTopic(topics, TopicRunCompleted) {
		t.Errorf("ai.run.completed not published; topics: %v", topics)
	}
}

// TestYieldedSession_WPOverridesAgentMaxSeconds verifies that work-plan
// session_max_seconds overrides the agent default.
func TestYieldedSession_WPOverridesAgentMaxSeconds(t *testing.T) {
	dm := newFakeDM()
	srv := makeHangingStreamServer(t) // keeps SSE connection open
	defer srv.Close()

	// Agent: 300s session limit, 2 sessions; WP: 1s session limit.
	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 300, 0, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 1, 0, 0) // WP overrides to 1s

	mgr, _ := newTestManager(dm)
	start := time.Now()
	mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {}) //nolint:errcheck
	elapsed := time.Since(start)

	// Should yield after ~1s (WP override), not after 300s (agent default).
	firstRun, _ := mgr.GetRun(context.Background(), runID)
	if firstRun.Status != AgentRunStatusYielded {
		t.Errorf("expected yielded, got %s", firstRun.Status)
	}
	// Generous bound: must finish well under the agent default of 300s.
	if elapsed > 10*time.Second {
		t.Errorf("took %v, WP override of 1s not respected", elapsed)
	}
}

// TestYieldedSession_WPPartialOverride verifies that a work-plan can override
// only one session field while agent defaults apply to the rest.
func TestYieldedSession_WPPartialOverride(t *testing.T) {
	// Agent: max_seconds=10, max_tokens=0, max_sessions=2
	// WP:    max_seconds=0 (use agent), max_tokens=5, max_sessions=0 (use agent)
	// Effective: max_seconds=10, max_tokens=5, max_sessions=2
	agent := Agent{
		SessionMaxSeconds:  10,
		SessionMaxTokens:   0,
		SessionMaxSessions: 2,
	}
	limits := resolveSessionLimits(agent, 0, 5, 0)
	if limits.MaxSeconds != 10 {
		t.Errorf("MaxSeconds: got %d, want 10", limits.MaxSeconds)
	}
	if limits.MaxTokens != 5 {
		t.Errorf("MaxTokens: got %d, want 5", limits.MaxTokens)
	}
	if limits.MaxSessions != 2 {
		t.Errorf("MaxSessions: got %d, want 2", limits.MaxSessions)
	}
}

// TestYieldedSession_MaxSessions1IsBackwardCompat verifies that the default
// max_sessions=1 behaviour (no yield) is preserved: a timeout produces
// ai.task.failed without going through the yield path.
func TestYieldedSession_MaxSessions1IsBackwardCompat(t *testing.T) {
	dm := newFakeDM()
	// Agent has no session limits set (defaults: max_sessions=1 → no yield).
	_, agentID := seedAgentWithProvider(t, dm, makeSlowServer(t).URL)
	runID := seedRunInPendingIntake(t, dm, agentID)

	mgr, pub := newTestManager(dm)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := mgr.ExecuteRunStreaming(ctx, runID, nil, func(string) {})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	storedRun, _ := mgr.GetRun(context.Background(), runID)
	if storedRun.Status != AgentRunStatusFailed {
		t.Errorf("expected failed, got %s", storedRun.Status)
	}
	if containsTopic(pub.published(), TopicTaskYielded) {
		t.Error("ai.task.yielded must NOT be published when max_sessions=1")
	}
	if !containsTopic(pub.published(), TopicTaskFailed) {
		t.Errorf("ai.task.failed must be published; topics: %v", pub.published())
	}
}

// TestYieldedSession_Session2ReceivesHistory verifies that the LLM request
// for session 2 contains session 1's partial output in the message history.
func TestYieldedSession_Session2ReceivesHistory(t *testing.T) {
	dm := newFakeDM()
	session1Output := strings.Repeat("s1chunk", 12) // 84 chars → 21 tokens

	var capturedBodies []string
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := int(callCount.Add(1))
		body, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, string(body))

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if call == 1 {
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": session1Output}}},
			})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		} else {
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": "done"}}},
			})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		}
	}))
	defer srv.Close()

	_, agentID := seedAgentWithSessionLimits(t, dm, srv.URL, 0, 5, 2)
	runID := seedRunWithWPOverrides(t, dm, agentID, 0, 0, 0)

	mgr, _ := newTestManager(dm)
	mgr.ExecuteRunStreaming(context.Background(), runID, nil, func(string) {}) //nolint:errcheck

	if len(capturedBodies) < 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(capturedBodies))
	}
	// Session 2 request body must contain session 1's partial output.
	if !strings.Contains(capturedBodies[1], session1Output) {
		t.Errorf("session 2 LLM request does not contain session 1 partial output")
	}
}

// TestResolveSessionLimits_Defaults verifies that zero agent values produce
// the documented defaults.
func TestResolveSessionLimits_Defaults(t *testing.T) {
	limits := resolveSessionLimits(Agent{}, 0, 0, 0)
	if limits.MaxSeconds != 300 {
		t.Errorf("default MaxSeconds: got %d, want 300", limits.MaxSeconds)
	}
	if limits.MaxTokens != 0 {
		t.Errorf("default MaxTokens: got %d, want 0", limits.MaxTokens)
	}
	if limits.MaxSessions != 1 {
		t.Errorf("default MaxSessions: got %d, want 1", limits.MaxSessions)
	}
}

// TestResolveSessionLimits_MaxSessionsMinOne ensures MaxSessions is clamped to 1.
func TestResolveSessionLimits_MaxSessionsMinOne(t *testing.T) {
	limits := resolveSessionLimits(Agent{SessionMaxSessions: -3}, 0, 0, 0)
	if limits.MaxSessions < 1 {
		t.Errorf("MaxSessions must be >= 1, got %d", limits.MaxSessions)
	}
}

// ── utility ───────────────────────────────────────────────────────────────────

func containsTopic(topics []string, target string) bool {
	for _, t := range topics {
		if t == target {
			return true
		}
	}
	return false
}

// lastTopicMatching returns the last topic from topics that is in candidates,
// or "" if none match.
func lastTopicMatching(topics []string, candidates ...string) string {
	set := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		set[c] = true
	}
	last := ""
	for _, t := range topics {
		if set[t] {
			last = t
		}
	}
	return last
}

func intVal(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
