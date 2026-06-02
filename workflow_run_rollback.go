// workflow_run_rollback.go — CodeValdAI leg of the WorkflowRun rollback
// coordinator (FEAT-20260602-004 Phase 2 — AI service).
//
// CodeValdWork's coordinator calls RollbackByWorkflowRun on every service in
// the affected closure. The AI rule from the FEAT spec is:
//
//   - In-flight AgentRuns (pending_intake, pending_execution, running, yielded)
//     transition to AgentRunStatusCancelled.
//   - Completed or failed AgentRuns transition to AgentRunStatusRolledBack as
//     a frozen audit record — LLM artefacts are preserved for debugging.
//   - Already-cancelled / already-rolled_back runs are skipped (idempotent).
package codevaldai

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// RollbackByWorkflowRun implements [AIManager.RollbackByWorkflowRun].
func (m *aiManager) RollbackByWorkflowRun(ctx context.Context, workflowRunID, reason string) (RollbackByWorkflowRunResult, error) {
	if workflowRunID == "" {
		return RollbackByWorkflowRunResult{}, ErrWorkflowRunIDRequired
	}

	runs, err := m.ListRuns(ctx, RunFilter{WorkflowRunID: workflowRunID})
	if err != nil {
		return RollbackByWorkflowRunResult{}, fmt.Errorf("RollbackByWorkflowRun %s: %w", workflowRunID, err)
	}

	result := RollbackByWorkflowRunResult{WorkflowRunID: workflowRunID}
	now := time.Now().UTC().Format(time.RFC3339)

	for _, run := range runs {
		switch run.Status {
		case AgentRunStatusCancelled, AgentRunStatusRolledBack:
			result.SkippedRunIDs = append(result.SkippedRunIDs, run.ID)
			continue

		case AgentRunStatusPendingIntake,
			AgentRunStatusPendingExecution,
			AgentRunStatusRunning,
			AgentRunStatusYielded:
			previous := run.Status
			if err := m.applyTerminalStatus(ctx, run.ID, AgentRunStatusCancelled, reason, now); err != nil {
				return result, fmt.Errorf("RollbackByWorkflowRun %s: cancel run %s: %w", workflowRunID, run.ID, err)
			}
			result.CancelledRunIDs = append(result.CancelledRunIDs, run.ID)
			m.publishJSON(ctx, TopicRunCancelled, RunCancelledPayload{
				RunID:          run.ID,
				AgentID:        run.AgentID,
				WorkflowRunID:  workflowRunID,
				PreviousStatus: previous,
				Reason:         reason,
			})

		case AgentRunStatusCompleted, AgentRunStatusFailed:
			previous := run.Status
			if err := m.applyTerminalStatus(ctx, run.ID, AgentRunStatusRolledBack, reason, now); err != nil {
				return result, fmt.Errorf("RollbackByWorkflowRun %s: rollback run %s: %w", workflowRunID, run.ID, err)
			}
			result.RolledBackRunIDs = append(result.RolledBackRunIDs, run.ID)
			m.publishJSON(ctx, TopicRunRolledBack, RunRolledBackPayload{
				RunID:          run.ID,
				AgentID:        run.AgentID,
				WorkflowRunID:  workflowRunID,
				PreviousStatus: previous,
				Reason:         reason,
			})

		default:
			// Unknown status — log and skip to avoid corrupting an entity in
			// an unexpected state.
			log.Printf("codevaldai: RollbackByWorkflowRun %s: skipping run %s with unknown status %q",
				workflowRunID, run.ID, run.Status)
			result.SkippedRunIDs = append(result.SkippedRunIDs, run.ID)
		}
	}

	return result, nil
}

// applyTerminalStatus patches the run entity with the new terminal status,
// completed_at (preserving any existing value), updated_at, and a
// rollback_reason field. Other fields (output, error_message, token counts)
// are intentionally left untouched so the audit trail is preserved.
func (m *aiManager) applyTerminalStatus(ctx context.Context, runID string, next AgentRunStatus, reason, now string) error {
	patch := map[string]any{
		"status":     string(next),
		"updated_at": now,
	}
	if reason != "" {
		patch["rollback_reason"] = reason
	}

	current, err := m.dm.GetEntity(ctx, m.agencyID, runID)
	if err != nil {
		return fmt.Errorf("get entity: %w", err)
	}
	if current.Properties["completed_at"] == nil || current.Properties["completed_at"] == "" {
		patch["completed_at"] = now
	}

	if _, err := m.dm.UpdateEntity(ctx, m.agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: patch,
	}); err != nil {
		return fmt.Errorf("update entity: %w", err)
	}
	return nil
}
