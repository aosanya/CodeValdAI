# FEAT-20260602-004 — CodeValdAI leg of WorkflowRun rollback (`DELETE /by-workflow-run/{id}`)

**Status:** ✅ Shipped — CodeValdAI's portion of [CodeValdWork FEAT-20260602-004](../../../../CodeValdWork/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-004_workflow_run_rollback_semantics.md) Phase 2
**Owner:** CodeValdAI
**Branch:** `feature/Dev-AI-FEAT-20260602-004_rollback-by-workflow-run`

---

## Overview

When CodeValdWork's rollback coordinator runs (`POST /workflow-runs/{id}/rollback` in CodeValdWork), it calls every downstream service to compensate the artifacts that service produced under the rolled-back run. The AI rule from the FEAT spec:

> **CodeValdAI** — Cancel in-flight AgentRuns (status → `cancelled`). Keep completed AgentRuns as audit (status → `rolled_back`, frozen). Don't delete LLM artifacts — they're useful for debugging.

This doc captures the AI side of that contract.

---

## API

### gRPC

`AIService.RollbackByWorkflowRun(RollbackByWorkflowRunRequest) → RollbackByWorkflowRunResponse`

Request:

| Field | Type | Required | Description |
|---|---|---|---|
| `workflow_run_id` | string | yes | The `WorkflowRun.ID` whose AgentRuns must be compensated. Empty → `INVALID_ARGUMENT`. |
| `reason` | string | no | Operator rollback reason. Recorded on each affected AgentRun's `rollback_reason` property and forwarded in `ai.run.cancelled` / `ai.run.rolled_back` payloads. |

Response:

| Field | Type | Description |
|---|---|---|
| `workflow_run_id` | string | Echoes the request. |
| `cancelled_run_ids` | repeated string | Runs that were in-flight at rollback time and were transitioned to `cancelled`. |
| `rolled_back_run_ids` | repeated string | Runs that had already reached `completed` or `failed` and were transitioned to `rolled_back` (frozen audit). |
| `skipped_run_ids` | repeated string | Runs that were already in a rollback-terminal state (`cancelled` / `rolled_back`). Included so the call is observably idempotent. |

### gRPC status mapping

| Domain error | gRPC code |
|---|---|
| `ErrWorkflowRunIDRequired` | `INVALID_ARGUMENT` |
| `entitygraph` storage error | `INTERNAL` |

---

## Per-run transition rules

| Previous status | Action | Next status | Event |
|---|---|---|---|
| `pending_intake` | cancel | `cancelled` | `ai.run.cancelled` |
| `pending_execution` | cancel | `cancelled` | `ai.run.cancelled` |
| `running` | cancel | `cancelled` | `ai.run.cancelled` |
| `yielded` | cancel | `cancelled` | `ai.run.cancelled` |
| `completed` | freeze | `rolled_back` | `ai.run.rolled_back` |
| `failed` | freeze | `rolled_back` | `ai.run.rolled_back` |
| `cancelled` | skip | (unchanged) | — |
| `rolled_back` | skip | (unchanged) | — |

For every transition the manager patches: `status`, `updated_at`, `rollback_reason`, and `completed_at` (only if not already set). `output`, `error_message`, `input_tokens`, `output_tokens`, and `partial_output` are preserved — the audit value is in keeping the LLM artefacts available.

The implementation is in [`workflow_run_rollback.go`](../../../workflow_run_rollback.go) and exposed via the `AIManager.RollbackByWorkflowRun` interface method in [`ai.go`](../../../ai.go).

---

## Data model additions

### `AgentRunStatus` (new terminal values)

```go
AgentRunStatusCancelled  AgentRunStatus = "cancelled"
AgentRunStatusRolledBack AgentRunStatus = "rolled_back"
```

### Proto enum (mirrors Go)

```protobuf
AGENT_RUN_STATUS_CANCELLED   = 7;
AGENT_RUN_STATUS_ROLLED_BACK = 8;
```

### Entity property

```go
// rollback_reason: operator-supplied reason recorded when the run is
// transitioned to cancelled or rolled_back by the rollback coordinator.
{Name: "rollback_reason", Type: types.PropertyTypeString}
```

---

## Events

### `ai.run.cancelled`

Published once per AgentRun the coordinator transitions to `cancelled`.

```go
type RunCancelledPayload struct {
    RunID          string         `json:"run_id"`
    AgentID        string         `json:"agent_id,omitempty"`
    WorkflowRunID  string         `json:"workflow_run_id"`
    PreviousStatus AgentRunStatus `json:"previous_status"`
    Reason         string         `json:"reason,omitempty"`
}
```

### `ai.run.rolled_back`

Published once per AgentRun the coordinator freezes as audit.

```go
type RunRolledBackPayload struct {
    RunID          string         `json:"run_id"`
    AgentID        string         `json:"agent_id,omitempty"`
    WorkflowRunID  string         `json:"workflow_run_id"`
    PreviousStatus AgentRunStatus `json:"previous_status"`
    Reason         string         `json:"reason,omitempty"`
}
```

`PreviousStatus` lets consumers distinguish a `completed → rolled_back` audit freeze from a `failed → rolled_back` audit freeze without re-fetching the run.

---

## Idempotency

The call is idempotent at the per-run level: a second call after a successful one finds every run in `cancelled` or `rolled_back` and routes them all into `skipped_run_ids`. No events are republished for skipped runs.

This matches the CodeValdWork coordinator's retry-after-`rollback_failed` path: if Phase 2 succeeded for AI but failed for another service, re-running the rollback re-issues the AI call without producing duplicate transitions or events.

---

## Tests

| Layer | File |
|---|---|
| Domain (`AIManager.RollbackByWorkflowRun`) | [`workflow_run_rollback_test.go`](../../../workflow_run_rollback_test.go) |
| gRPC handler + status converter | [`internal/server/workflow_run_rollback_server_test.go`](../../../internal/server/workflow_run_rollback_server_test.go) |

Covered scenarios:

- Empty `workflow_run_id` → `ErrWorkflowRunIDRequired` / `INVALID_ARGUMENT`.
- No matching runs → empty result, no events.
- All four in-flight statuses → `cancelled`, one `ai.run.cancelled` per run.
- `completed` + `failed` → `rolled_back`, one `ai.run.rolled_back` per run.
- Already-`cancelled` and already-`rolled_back` runs → skipped, no events.
- Mixed closure (in-flight + terminal + skip) → result partitioned correctly; runs anchored to a different `workflow_run_id` are untouched.
- Idempotent retry — second call returns runs in `skipped_run_ids`.
- `reason` is recorded on the run's `rollback_reason` property.
- gRPC status conversions for the two new enum values round-trip cleanly.
- Manager errors surface as `INTERNAL`.

---

## Open follow-ups

1. **CodeValdWork coordinator wiring** — `stubCompensateAI` in [`CodeValdWork/workflow_run_rollback.go`](../../../../CodeValdWork/workflow_run_rollback.go) still logs and returns. Replace with a gRPC call to `RollbackByWorkflowRun` so the end-to-end coordinator actually invokes the AI leg.
2. **State machine guard** — execute/intake paths do not currently reject writes against `cancelled` / `rolled_back` runs. If a slow in-flight handler completes after the rollback hits, it could overwrite the cancelled status. Add a status check before any non-rollback status mutation. Tracked in this FEAT's follow-up rather than blocking the contract.
3. **Frontend surfacing** — the WorkFrontend run-detail view needs to render the two new statuses with distinct chips (cancelled vs rolled_back vs failed).
