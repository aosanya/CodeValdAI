package codevaldai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// seedRun inserts an AgentRun directly with the given status + workflow_run_id
// and links it to agentID via the belongs_to_agent edge. Returns the run ID.
func seedRun(t *testing.T, dm *fakeDataManager, agentID, workflowRunID string, status AgentRunStatus) string {
	t.Helper()
	dm.mu.Lock()
	defer dm.mu.Unlock()

	runID := dm.nextID()
	dm.entities[runID] = entitygraph.Entity{
		ID:       runID,
		AgencyID: testAgencyID,
		TypeID:   "AgentRun",
		Properties: map[string]any{
			"instructions":    "test",
			"status":          string(status),
			"workflow_run_id": workflowRunID,
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

func TestRollbackByWorkflowRun_EmptyID_ReturnsError(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.RollbackByWorkflowRun(context.Background(), "", "")
	if !errors.Is(err, ErrWorkflowRunIDRequired) {
		t.Errorf("err = %v want ErrWorkflowRunIDRequired", err)
	}
}

func TestRollbackByWorkflowRun_NoMatchingRuns_EmptyResult(t *testing.T) {
	mgr, pub := newTestManager(newFakeDM())
	result, err := mgr.RollbackByWorkflowRun(context.Background(), "wf-empty", "reason")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if result.WorkflowRunID != "wf-empty" {
		t.Errorf("WorkflowRunID = %q want wf-empty", result.WorkflowRunID)
	}
	if len(result.CancelledRunIDs)+len(result.RolledBackRunIDs)+len(result.SkippedRunIDs) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
	if len(pub.published()) != 0 {
		t.Errorf("no events expected, got %v", pub.published())
	}
}

func TestRollbackByWorkflowRun_InFlightRuns_TransitionToCancelled(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)
	_, agentID := seedAgentWithProvider(t, dm, "")
	const wfID = "wf-inflight"

	inflight := []AgentRunStatus{
		AgentRunStatusPendingIntake,
		AgentRunStatusPendingExecution,
		AgentRunStatusRunning,
		AgentRunStatusYielded,
	}
	var runIDs []string
	for _, s := range inflight {
		runIDs = append(runIDs, seedRun(t, dm, agentID, wfID, s))
	}

	result, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "regression")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if len(result.CancelledRunIDs) != len(runIDs) {
		t.Errorf("CancelledRunIDs = %v want %d entries", result.CancelledRunIDs, len(runIDs))
	}
	if len(result.RolledBackRunIDs) != 0 {
		t.Errorf("RolledBackRunIDs should be empty, got %v", result.RolledBackRunIDs)
	}

	for _, id := range runIDs {
		run, err := mgr.GetRun(context.Background(), id)
		if err != nil {
			t.Fatalf("GetRun %s: %v", id, err)
		}
		if run.Status != AgentRunStatusCancelled {
			t.Errorf("run %s status = %q want cancelled", id, run.Status)
		}
	}

	// One ai.run.cancelled event per cancelled run.
	cancelledCount := 0
	for _, topic := range pub.published() {
		if topic == TopicRunCancelled {
			cancelledCount++
		}
	}
	if cancelledCount != len(runIDs) {
		t.Errorf("ai.run.cancelled count = %d want %d", cancelledCount, len(runIDs))
	}
}

func TestRollbackByWorkflowRun_TerminalRuns_TransitionToRolledBack(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)
	_, agentID := seedAgentWithProvider(t, dm, "")
	const wfID = "wf-terminal"

	completedID := seedRun(t, dm, agentID, wfID, AgentRunStatusCompleted)
	failedID := seedRun(t, dm, agentID, wfID, AgentRunStatusFailed)

	result, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if len(result.RolledBackRunIDs) != 2 {
		t.Errorf("RolledBackRunIDs = %v want 2 entries", result.RolledBackRunIDs)
	}
	if len(result.CancelledRunIDs) != 0 {
		t.Errorf("CancelledRunIDs should be empty, got %v", result.CancelledRunIDs)
	}

	for _, id := range []string{completedID, failedID} {
		run, err := mgr.GetRun(context.Background(), id)
		if err != nil {
			t.Fatalf("GetRun %s: %v", id, err)
		}
		if run.Status != AgentRunStatusRolledBack {
			t.Errorf("run %s status = %q want rolled_back", id, run.Status)
		}
	}

	rolledBackCount := 0
	for _, topic := range pub.published() {
		if topic == TopicRunRolledBack {
			rolledBackCount++
		}
	}
	if rolledBackCount != 2 {
		t.Errorf("ai.run.rolled_back count = %d want 2", rolledBackCount)
	}
}

func TestRollbackByWorkflowRun_AlreadyRolledBack_Skipped(t *testing.T) {
	dm := newFakeDM()
	mgr, pub := newTestManager(dm)
	_, agentID := seedAgentWithProvider(t, dm, "")
	const wfID = "wf-skip"

	cancelledID := seedRun(t, dm, agentID, wfID, AgentRunStatusCancelled)
	rolledID := seedRun(t, dm, agentID, wfID, AgentRunStatusRolledBack)

	result, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if len(result.SkippedRunIDs) != 2 {
		t.Errorf("SkippedRunIDs = %v want 2 entries", result.SkippedRunIDs)
	}
	containsID := func(ids []string, id string) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	if !containsID(result.SkippedRunIDs, cancelledID) || !containsID(result.SkippedRunIDs, rolledID) {
		t.Errorf("SkippedRunIDs %v missing one of %s,%s", result.SkippedRunIDs, cancelledID, rolledID)
	}
	if len(pub.published()) != 0 {
		t.Errorf("no events expected for skipped runs, got %v", pub.published())
	}
}

func TestRollbackByWorkflowRun_MixedClosure_PartitionsCorrectly(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)
	_, agentID := seedAgentWithProvider(t, dm, "")
	const wfID = "wf-mixed"

	runningID := seedRun(t, dm, agentID, wfID, AgentRunStatusRunning)
	completedID := seedRun(t, dm, agentID, wfID, AgentRunStatusCompleted)
	cancelledID := seedRun(t, dm, agentID, wfID, AgentRunStatusCancelled)
	// Different wf — must be ignored.
	otherID := seedRun(t, dm, agentID, "wf-other", AgentRunStatusRunning)

	result, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "mix")
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if len(result.CancelledRunIDs) != 1 || result.CancelledRunIDs[0] != runningID {
		t.Errorf("CancelledRunIDs = %v want [%s]", result.CancelledRunIDs, runningID)
	}
	if len(result.RolledBackRunIDs) != 1 || result.RolledBackRunIDs[0] != completedID {
		t.Errorf("RolledBackRunIDs = %v want [%s]", result.RolledBackRunIDs, completedID)
	}
	if len(result.SkippedRunIDs) != 1 || result.SkippedRunIDs[0] != cancelledID {
		t.Errorf("SkippedRunIDs = %v want [%s]", result.SkippedRunIDs, cancelledID)
	}

	// Run anchored to the other workflow must remain untouched.
	other, err := mgr.GetRun(context.Background(), otherID)
	if err != nil {
		t.Fatalf("GetRun other: %v", err)
	}
	if other.Status != AgentRunStatusRunning {
		t.Errorf("other run status = %q want still running", other.Status)
	}
}

func TestRollbackByWorkflowRun_Idempotent(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)
	_, agentID := seedAgentWithProvider(t, dm, "")
	const wfID = "wf-idem"
	runID := seedRun(t, dm, agentID, wfID, AgentRunStatusRunning)

	first, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(first.CancelledRunIDs) != 1 {
		t.Fatalf("first.CancelledRunIDs = %v want 1", first.CancelledRunIDs)
	}

	second, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(second.CancelledRunIDs) != 0 {
		t.Errorf("second.CancelledRunIDs = %v want empty (idempotent)", second.CancelledRunIDs)
	}
	if len(second.SkippedRunIDs) != 1 || second.SkippedRunIDs[0] != runID {
		t.Errorf("second.SkippedRunIDs = %v want [%s]", second.SkippedRunIDs, runID)
	}
}

func TestRollbackByWorkflowRun_RecordsReasonOnEntity(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)
	_, agentID := seedAgentWithProvider(t, dm, "")
	const wfID = "wf-reason"
	runID := seedRun(t, dm, agentID, wfID, AgentRunStatusRunning)

	if _, err := mgr.RollbackByWorkflowRun(context.Background(), wfID, "manual intervention"); err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}

	dm.mu.Lock()
	got, _ := dm.entities[runID].Properties["rollback_reason"].(string)
	dm.mu.Unlock()
	if !strings.Contains(got, "manual intervention") {
		t.Errorf("rollback_reason = %q want substring 'manual intervention'", got)
	}
}
