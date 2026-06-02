package codevaldai

import (
	"context"
	"testing"
)

// TestIntakeRun_PersistsWorkflowRunID covers FEAT-20260602-001 (AI):
// when the caller supplies WorkflowRunID, IntakeRun stores it on the AgentRun
// entity so downstream events and the closure SSE can find the run.
func TestIntakeRun_PersistsWorkflowRunID(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	fieldJSON := `[]`
	srv := makeOpenAISSEServer(t, fieldJSON)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	const wantWFR = "wfr-abc-123"
	const wantTask = "task-7"
	run, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:       agentID,
		Instructions:  "do the thing",
		TaskID:        wantTask,
		WorkflowRunID: wantWFR,
	})
	if err != nil {
		t.Fatalf("IntakeRun: %v", err)
	}
	if run.WorkflowRunID != wantWFR {
		t.Errorf("run.WorkflowRunID = %q, want %q", run.WorkflowRunID, wantWFR)
	}

	got, err := mgr.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.WorkflowRunID != wantWFR {
		t.Errorf("persisted WorkflowRunID = %q, want %q", got.WorkflowRunID, wantWFR)
	}
	if got.TaskID != wantTask {
		t.Errorf("persisted TaskID = %q, want %q", got.TaskID, wantTask)
	}
}

// TestIntakeRun_EmptyWorkflowRunID confirms that manually-triggered runs
// (no caller-supplied id) persist as orphans — the field is empty, not unset.
// Per the umbrella §Open design questions, orphaned runs are allowed for v1.
func TestIntakeRun_EmptyWorkflowRunID(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	srv := makeOpenAISSEServer(t, `[]`)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	run, _, err := mgr.IntakeRun(context.Background(), IntakeRunRequest{
		AgentID:      agentID,
		Instructions: "manual run",
	})
	if err != nil {
		t.Fatalf("IntakeRun: %v", err)
	}
	if run.WorkflowRunID != "" {
		t.Errorf("manual run WorkflowRunID = %q, want empty", run.WorkflowRunID)
	}
}

// TestListRuns_WorkflowRunIDFilter covers the closure SSE call path:
// GET /ai/{agency}/runs?workflow_run_id=X must return only runs from that run.
func TestListRuns_WorkflowRunIDFilter(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)

	srv := makeOpenAISSEServer(t, `[]`)
	defer srv.Close()
	_, agentID := seedAgentWithProvider(t, dm, srv.URL)

	const wfrA = "wfr-alpha"
	const wfrB = "wfr-beta"

	ctx := context.Background()
	for i, wfr := range []string{wfrA, wfrA, wfrB, ""} {
		_, _, err := mgr.IntakeRun(ctx, IntakeRunRequest{
			AgentID:       agentID,
			Instructions:  "task",
			WorkflowRunID: wfr,
		})
		if err != nil {
			t.Fatalf("IntakeRun[%d]: %v", i, err)
		}
	}

	gotA, err := mgr.ListRuns(ctx, RunFilter{WorkflowRunID: wfrA})
	if err != nil {
		t.Fatalf("ListRuns wfr=A: %v", err)
	}
	if len(gotA) != 2 {
		t.Errorf("ListRuns wfr=%q returned %d runs, want 2", wfrA, len(gotA))
	}
	for _, r := range gotA {
		if r.WorkflowRunID != wfrA {
			t.Errorf("ListRuns wfr=%q returned run with WorkflowRunID=%q", wfrA, r.WorkflowRunID)
		}
	}

	gotB, err := mgr.ListRuns(ctx, RunFilter{WorkflowRunID: wfrB})
	if err != nil {
		t.Fatalf("ListRuns wfr=B: %v", err)
	}
	if len(gotB) != 1 {
		t.Errorf("ListRuns wfr=%q returned %d runs, want 1", wfrB, len(gotB))
	}

	all, err := mgr.ListRuns(ctx, RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns no filter: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("ListRuns no filter returned %d runs, want 4", len(all))
	}
}
