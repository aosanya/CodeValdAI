package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	codevaldai "github.com/aosanya/CodeValdAI"
	agencypb "github.com/aosanya/CodeValdAgency/gen/go/codevaldagency/v1"
	"google.golang.org/grpc"
)

// ── fakeRolesMatcher ──────────────────────────────────────────────────────────

type fakeRolesMatcher struct {
	resp *agencypb.MatchRolesResponse
	err  error
}

func (f *fakeRolesMatcher) MatchRoles(_ context.Context, _ *agencypb.MatchRolesRequest, _ ...grpc.CallOption) (*agencypb.MatchRolesResponse, error) {
	return f.resp, f.err
}

// ── fakeAIManager ─────────────────────────────────────────────────────────────

type fakeAIManager struct {
	mu         sync.Mutex
	intakeCalls []codevaldai.IntakeRunRequest
	executeCalls []string // run IDs
	intakeErr  error
	executeErr error
	runID      string
}

func (f *fakeAIManager) IntakeRun(_ context.Context, req codevaldai.IntakeRunRequest) (codevaldai.AgentRun, []codevaldai.RunField, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intakeCalls = append(f.intakeCalls, req)
	if f.intakeErr != nil {
		return codevaldai.AgentRun{}, nil, f.intakeErr
	}
	id := f.runID
	if id == "" {
		id = "run-1"
	}
	return codevaldai.AgentRun{ID: id}, nil, nil
}

func (f *fakeAIManager) ExecuteRunStreaming(_ context.Context, runID string, _ []codevaldai.RunInput, _ func(string)) (codevaldai.AgentRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executeCalls = append(f.executeCalls, runID)
	return codevaldai.AgentRun{ID: runID}, f.executeErr
}

// Unused AIManager methods — satisfy the interface.
func (f *fakeAIManager) CreateProvider(_ context.Context, _ codevaldai.CreateProviderRequest) (codevaldai.LLMProvider, error) {
	return codevaldai.LLMProvider{}, nil
}
func (f *fakeAIManager) GetProvider(_ context.Context, _ string) (codevaldai.LLMProvider, error) {
	return codevaldai.LLMProvider{}, nil
}
func (f *fakeAIManager) ListProviders(_ context.Context) ([]codevaldai.LLMProvider, error) {
	return nil, nil
}
func (f *fakeAIManager) UpdateProvider(_ context.Context, _ string, _ codevaldai.UpdateProviderRequest) (codevaldai.LLMProvider, error) {
	return codevaldai.LLMProvider{}, nil
}
func (f *fakeAIManager) DeleteProvider(_ context.Context, _ string) error { return nil }
func (f *fakeAIManager) CreateAgent(_ context.Context, _ codevaldai.CreateAgentRequest) (codevaldai.Agent, error) {
	return codevaldai.Agent{}, nil
}
func (f *fakeAIManager) GetAgent(_ context.Context, _ string) (codevaldai.Agent, error) {
	return codevaldai.Agent{}, nil
}
func (f *fakeAIManager) ListAgents(_ context.Context) ([]codevaldai.Agent, error) { return nil, nil }
func (f *fakeAIManager) UpdateAgent(_ context.Context, _ string, _ codevaldai.UpdateAgentRequest) (codevaldai.Agent, error) {
	return codevaldai.Agent{}, nil
}
func (f *fakeAIManager) DeleteAgent(_ context.Context, _ string) error { return nil }
func (f *fakeAIManager) ExecuteRun(_ context.Context, _ string, _ []codevaldai.RunInput) (codevaldai.AgentRun, error) {
	return codevaldai.AgentRun{}, nil
}
func (f *fakeAIManager) GetRun(_ context.Context, _ string) (codevaldai.AgentRun, error) {
	return codevaldai.AgentRun{}, nil
}
func (f *fakeAIManager) ListRuns(_ context.Context, _ codevaldai.RunFilter) ([]codevaldai.AgentRun, error) {
	return nil, nil
}

// ── RACIDispatcher tests ──────────────────────────────────────────────────────

func TestRACIDispatcher_Dispatch_TriggersRunForMatchedRole(t *testing.T) {
	t.Parallel()
	client := &fakeRolesMatcher{
		resp: &agencypb.MatchRolesResponse{
			Matches: []*agencypb.RoleMatch{
				{
					Role: &agencypb.Role{
						Id:           "role-1",
						AgentId:      "agent-42",
						Instructions: "Analyze the task change.",
					},
				},
			},
		},
	}
	mgr := &fakeAIManager{runID: "run-99"}
	d := NewRACIDispatcher(client, mgr, "agency-1")

	// Run synchronously by using the unexported triggerRoleRun directly.
	match := client.resp.GetMatches()[0]
	err := d.triggerRoleRun(context.Background(), match, "work.task.status.changed", `{"task_id":"t1"}`)
	if err != nil {
		t.Fatalf("triggerRoleRun: %v", err)
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.intakeCalls) != 1 {
		t.Fatalf("expected 1 IntakeRun call, got %d", len(mgr.intakeCalls))
	}
	req := mgr.intakeCalls[0]
	if req.AgentID != "agent-42" {
		t.Errorf("AgentID: want %q, got %q", "agent-42", req.AgentID)
	}
	if len(mgr.executeCalls) != 1 {
		t.Fatalf("expected 1 ExecuteRunStreaming call, got %d", len(mgr.executeCalls))
	}
	if mgr.executeCalls[0] != "run-99" {
		t.Errorf("execute run ID: want %q, got %q", "run-99", mgr.executeCalls[0])
	}
}

func TestRACIDispatcher_Dispatch_SkipsRoleWithNoAgentID(t *testing.T) {
	t.Parallel()
	client := &fakeRolesMatcher{
		resp: &agencypb.MatchRolesResponse{
			Matches: []*agencypb.RoleMatch{
				{Role: &agencypb.Role{Id: "role-1", AgentId: ""}},
			},
		},
	}
	mgr := &fakeAIManager{}
	d := NewRACIDispatcher(client, mgr, "agency-1")

	match := client.resp.GetMatches()[0]
	if err := d.triggerRoleRun(context.Background(), match, "work.task.status.changed", "{}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.intakeCalls) != 0 {
		t.Errorf("expected 0 IntakeRun calls, got %d", len(mgr.intakeCalls))
	}
}

func TestRACIDispatcher_Dispatch_MatchRolesError_NoRun(t *testing.T) {
	t.Parallel()
	client := &fakeRolesMatcher{err: errors.New("agency unavailable")}
	mgr := &fakeAIManager{}
	d := NewRACIDispatcher(client, mgr, "agency-1")

	d.Dispatch(context.Background(), "work.task.status.changed", "{}")

	// Dispatch is async — give goroutines a moment (none should fire here).
	time.Sleep(20 * time.Millisecond)
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.intakeCalls) != 0 {
		t.Errorf("expected 0 IntakeRun calls on MatchRoles error, got %d", len(mgr.intakeCalls))
	}
}

func TestRACIDispatcher_Dispatch_NoMatches_NoRun(t *testing.T) {
	t.Parallel()
	client := &fakeRolesMatcher{resp: &agencypb.MatchRolesResponse{}}
	mgr := &fakeAIManager{}
	d := NewRACIDispatcher(client, mgr, "agency-1")

	d.Dispatch(context.Background(), "some.other.topic", "{}")

	time.Sleep(20 * time.Millisecond)
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.intakeCalls) != 0 {
		t.Errorf("expected 0 IntakeRun calls on empty matches, got %d", len(mgr.intakeCalls))
	}
}

// ── buildDispatchInstructions tests ──────────────────────────────────────────

func TestBuildDispatchInstructions_IncludesTopicAndPayload(t *testing.T) {
	t.Parallel()
	role := &agencypb.Role{Instructions: "Do the analysis."}
	result := buildDispatchInstructions(role, nil, "work.task.status.changed", `{"task_id":"t1"}`)

	if result == "" {
		t.Fatal("expected non-empty instructions")
	}
	for _, want := range []string{
		"Do the analysis.",
		"work.task.status.changed",
		`{"task_id":"t1"}`,
	} {
		if !contains(result, want) {
			t.Errorf("instructions missing %q", want)
		}
	}
}

func TestBuildDispatchInstructions_NoRoleInstructions_StillIncludesEvent(t *testing.T) {
	t.Parallel()
	role := &agencypb.Role{}
	result := buildDispatchInstructions(role, nil, "work.task.status.changed", `{"x":1}`)
	if !contains(result, "work.task.status.changed") {
		t.Error("missing topic")
	}
	if !contains(result, `{"x":1}`) {
		t.Error("missing payload")
	}
}

func TestBuildDispatchInstructions_GitContextSource_IncludesSignals(t *testing.T) {
	t.Parallel()
	role := &agencypb.Role{}
	sources := []*agencypb.ContextSource{
		{
			SourceType: "GitContextSource",
			Git:        &agencypb.GitContextSource{Signals: "commit,pr"},
		},
	}
	result := buildDispatchInstructions(role, sources, "t", "p")
	if !contains(result, "GitContextSource") {
		t.Error("missing source type")
	}
	if !contains(result, "commit,pr") {
		t.Error("missing signals")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
