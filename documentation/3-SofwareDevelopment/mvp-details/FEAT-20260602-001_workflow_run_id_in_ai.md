# FEAT-20260602-001 — `workflow_run_id` propagation in CodeValdAI

**Status:** 📋 Not Started
**Severity:** High — sibling of the umbrella; AgentRuns are the largest single artifact class produced during a pipeline (decomposition runs + per-todo execution runs) and are currently invisible to the WorkflowRun closure
**Owner:** CodeValdAI
**Estimated effort:** ~1.5 days (schema + proto + run intake propagation + list filter + integration tests)
**Source finding:** This conversation (2026-06-02) — sibling of [umbrella FEAT-20260602-001 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md)

---

## Problem

CodeValdAI creates an `AgentRun` for every dispatched run (decomposition, per-todo execution, diagnostic runs). Today these rows have no link back to the originating `WorkflowRun`, so:

- The closure view at `/agencies/.../workflow-runs/{id}` cannot show which AgentRuns belong to the run.
- Rollback cannot stop in-flight runs that were dispatched by the same pipeline.
- Operators debugging a failed run have to manually correlate AgentRun timestamps with the pipeline timeline.

## Goal

Make `workflow_run_id` a first-class typed field on:

- `AgentRun` entity
- Every `ai.run.*` event payload (`ai.run.requested`, `ai.run.started`, `ai.run.completed`, `ai.run.failed`, `ai.run.yielded`)
- `ListAgentRuns` RPC / `GET /ai/{agencyId}/runs` HTTP route, with `?workflow_run_id=X` filter

## Non-goals

- Backfilling existing AgentRun rows.
- Adding `workflow_run_id` to LLM provider configs, agents, or other config entities (out of scope per the [umbrella](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md)).

---

## Design

### Schema change

In `schema.go`, under the `AgentRun` `TypeDefinition`:

```go
{Name: "workflow_run_id", Type: types.PropertyTypeString},
```

### Proto change

In `proto/codevaldai/v1/`:

- `AgentRun` message: `string workflow_run_id = N;`
- `DispatchRunRequest` accepts `string workflow_run_id` (optional; usually inherited from the inbound triggering event).
- `ListAgentRunsRequest` accepts `string workflow_run_id` filter.

### Event payload changes

Every event in [`internal/server/run_yielded.go`](../../internal/server/run_yielded.go), [`internal/server/run_execution.go`](../../internal/server/run_execution.go), [`internal/server/run_intake.go`](../../internal/server/run_intake.go) (or wherever the publish helpers live) gains `workflow_run_id`.

### Chain-through behaviour

The `DispatchRun` entry point is the chain-through hot path:

- When called from a work-plan trigger (e.g. `developer-assigned-handler` reacting to `work.task.assigned`), the trigger event carries `workflow_run_id` — propagate onto the new `AgentRun`.
- When called manually (CLI / direct API), `workflow_run_id` is optional — if empty, the AgentRun is orphaned (allowed for v1 per umbrella §Open design questions).
- When the AgentRun calls back to CodeValdWork to create TaskTodos (decomposition), include `workflow_run_id` in the create request — Work uses it on the new todo rows.
- When the AgentRun completes/fails, the published event includes `workflow_run_id`.

### List filter

`GET /ai/{agencyId}/runs?workflow_run_id=X` returns all AgentRuns the run produced. The closure SSE endpoint ([FEAT-20260602-003 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-003_workflow_run_closure_sse_aggregation.md)) calls this.

---

## Implementation plan

### Phase 1 — Schema + proto (~0.5 day)

1. Add property to `AgentRun` in `schema.go`.
2. Add proto field; `make proto`.

### Phase 2 — Run intake + execution (~0.5 day)

1. Read `workflow_run_id` from the trigger event in `run_intake.go` and persist on the new AgentRun.
2. Pass `workflow_run_id` on every callback into CodeValdWork (create todos, complete todos).
3. Include in every published event payload.

### Phase 3 — Tests (~0.5 day)

- Unit: dispatch with run-id → AgentRun persists it; without → empty.
- Integration: pipeline ⇒ AgentRun with run-id ⇒ decomposed todos carry the same run-id.

---

## Verification

- `go test -race -count=1 ./...` clean.
- Run scenario 09; `GET /ai/utility-app-builder/runs?workflow_run_id=$RUN` returns the decomposition + per-todo AgentRuns.
- Each `ai.run.completed` event in the SSE log carries `workflow_run_id`.

---

## Open design questions

1. **Diagnostic runs.** When `merge-failure-diagnostics` ([scenario-09/00-setup Step 14](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/09/00-setup.md)) fires a *new* AgentRun to diagnose a failed merge, does that diagnostic run belong to the original pipeline (same `workflow_run_id`) or to a fresh one? Recommend same — the failure is part of the original pipeline closure.
2. **`ai.run.yielded`.** A long-running AgentRun emits periodic `yielded` events. Each must include `workflow_run_id` (cheap, no extra DB lookup if read from the entity once at start).

---

## Dependencies

- Part of umbrella: [FEAT-20260602-001 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md).
- Pairs with: [Work sibling FEAT](../../../../CodeValdWork/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-002_workflow_run_id_in_work.md) — Work passes the run-id into `DispatchRun`; AI persists it; AI passes it back on todo callbacks.
