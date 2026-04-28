// Package recovery provides startup reconciliation for AgentRun entities
// left in non-terminal states by a previous service restart.
package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	codevaldai "github.com/aosanya/CodeValdAI"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// Publisher is the minimal CrossPublisher contract needed by the boot sweep.
// codevaldai.CrossPublisher satisfies it implicitly; tests can inject a fake.
type Publisher interface {
	Publish(ctx context.Context, topic string, agencyID string) error
}

// ReconcileRunningRuns transitions any AgentRun left in "running" state to
// "failed" with error_message="interrupted by service restart" and publishes
// "cross.ai.{agencyID}.run.failed" for each. Called once on startup before
// the gRPC server begins accepting requests.
//
// Per-run failures are logged and skipped — one bad row must not block the
// rest of the sweep. A nil Publisher disables event publishing (a Publisher
// failure is logged but never returned).
//
// MULTI-REPLICA WARNING: this implementation assumes a single replica. With
// multiple replicas, the sweep will incorrectly fail another replica's
// in-flight runs. See documentation/3-SofwareDevelopment/mvp-details/
// llm-client/dispatcher.md for the documented future-work mitigation.
func ReconcileRunningRuns(
	ctx context.Context,
	dm entitygraph.DataManager,
	publisher Publisher,
	agencyID string,
	logger *slog.Logger,
) error {
	if logger == nil {
		logger = slog.Default()
	}

	runs, err := dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID:   agencyID,
		TypeID:     "AgentRun",
		Properties: map[string]any{"status": string(codevaldai.AgentRunStatusRunning)},
	})
	if err != nil {
		return fmt.Errorf("query running runs: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	topic := fmt.Sprintf("cross.ai.%s.run.failed", agencyID)
	for _, run := range runs {
		if _, err := dm.UpdateEntity(ctx, agencyID, run.ID, entitygraph.UpdateEntityRequest{
			Properties: map[string]any{
				"status":        string(codevaldai.AgentRunStatusFailed),
				"error_message": "interrupted by service restart",
				"completed_at":  now,
				"updated_at":    now,
			},
		}); err != nil {
			logger.Error("reconcile run", "run_id", run.ID, "err", err)
			continue
		}
		if publisher != nil {
			if err := publisher.Publish(ctx, topic, agencyID); err != nil {
				logger.Warn("publish run.failed during reconcile", "run_id", run.ID, "err", err)
			}
		}
		logger.Info("reconciled interrupted run", "run_id", run.ID)
	}
	return nil
}
