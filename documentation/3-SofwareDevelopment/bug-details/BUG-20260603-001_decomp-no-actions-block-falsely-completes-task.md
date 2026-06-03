# BUG-20260603-001 — Decomposition run with no actions block falsely completes the task

**Status:** 📋 Open
**Severity:** High — any task whose decomposition run produces no actions block (token cap, model refusal, parser failure) silently marks the task COMPLETED with zero todos; the pipeline halts with no error signal and the task cannot be re-run
**Owner:** CodeValdAI
**Estimated effort:** ~0.5 day (two targeted guards + config verification)
**Source finding:** QA scenario 09 run (2026-06-03) — decomp run `f253f4af-cdef-4aa6-a361-e934ad11563c` for MVP-SF-002 (Auth — Registration and Login)

## Problem

Two distinct failures, both present in the same run:

**Failure A — Token cap hit, no actions block produced:**
The model exhausted 4096 output tokens entirely inside `<think>` (reasoning through a 12-file Auth task). It produced zero content tokens — no post-think actions block, no in-think fallback block either. `dispatchActions` logged `no actions block in output` and returned cleanly.

**Failure B — CodeValdAI publishes `ai.task.completed` when actions are absent:**
Despite the empty output, `ExecuteRun` proceeded to publish `ai.task.completed`. CodeValdWork received this event and marked the task `TASK_STATUS_COMPLETED`. The task cannot be retried because its status is terminal.

These are related but separable. Failure A is a model/config issue; Failure B is a correctness bug — the service must not complete a task that was never decomposed.

## Evidence

```
# docker compose logs codevaldai (run f253f4af)
codevaldai: stream done: reason="length" reasonFrames=4096 contentFrames=0 inputTok=3357 outputTok=4096
codevaldai: ExecuteRun run=f253f4af ... llm ok: input_tokens=3357 output_tokens=4096 output_len=17728
codevaldai: dispatchActions: no actions block in output
codevaldai: registrar: publish topic="ai.task.completed" agencyID="utility-app-builder"

# Task status after the run
curl -s "${BASE}/work/utility-app-builder/tasks/73540b47-..." -u "$CV_AUTH" \
  | python3 -c "import sys,json; t=json.load(sys.stdin)['task']; print(t['status'])"
# TASK_STATUS_COMPLETED   ← falsely completed; zero todos exist
```

Note: `reasonFrames=4096 contentFrames=0` distinguishes this failure mode from BUG-09-028 (which had `contentFrames > 0` with a truncated post-think block). Phase 2 of BUG-09-028 raised `max_tokens` to 16384; if the running container has a stale binary, the old 4096-token limit is still active. Verify `developer-agent.max_tokens` in `CodeValdImplementations/Agencies/utility-app-builder/agency.json` is 16384 and that the container's binary is up to date.

## Root cause

### Failure A

`ExecuteRun` in `execute.go` uses the agent's `MaxTokens` value from the database. If the running container was built or seeded before BUG-09-028 Phase 2, `developer-agent.max_tokens` may still be 4096. A complex 12-file task consumes all reasoning tokens before the model can emit a structured actions block.

Even at 16384, a sufficiently complex task can reproduce this — the fix for Failure B is independently necessary.

### Failure B

`ExecuteRun` does not gate `ai.task.completed` on the presence of actions. The current flow (simplified):

```go
output, err := llm.Stream(...)
dispatchActions(output)          // logs "no actions block" but returns nil
publishEvent("ai.task.completed") // fires unconditionally
```

`dispatchActions` returning `nil` (no error) when no actions are found makes it look like a success.

## Fix plan

### Fix A — Verify and enforce max_tokens config

1. Read `CodeValdImplementations/Agencies/utility-app-builder/agency.json`. Confirm `developer-agent.max_tokens == 16384`. If not, update it (BUG-09-028 Phase 2 should have done this).
2. Add a startup log in `ExecuteRun`: `INFO: agent max_tokens=%d` so future runs are auditable.
3. Consider adding a minimum floor guard: if `agent.MaxTokens < 8192`, log WARN and clamp to 8192.

### Fix B — Treat no-actions-block as a run failure

In `execute.go` (or wherever `dispatchActions` is called), check its return:

```go
hasActions, err := dispatchActions(ctx, runID, agencyID, output)
if err != nil {
    publishEvent("ai.run.failed", ...)
    publishEvent("ai.task.failed", ...)
    return
}
if !hasActions {
    // Run produced output but no structured actions — treat as failure
    log.Printf("WARN: run=%s dispatchActions: no actions block; publishing ai.run.failed", runID)
    publishEvent("ai.run.failed", RunFailedPayload{RunID: runID, Reason: "no_actions_block"})
    publishEvent("ai.task.failed", TaskFailedPayload{TaskID: taskID, Reason: "decomp_no_actions"})
    return
}
publishEvent("ai.task.completed", ...)
```

`ai.task.failed` triggers CodeValdWork's failure-pipeline logic (watchdog, budget gate, etc.) rather than silently completing the task.

**Note on retry:** `ai.task.failed` in CodeValdWork should flip the task to `TASK_STATUS_FAILED` unless the failure budget allows a recovery pipeline. The `developer-assigned-handler` can be extended to re-dispatch when `work.task.failed` fires if the budget is not exhausted — but that is a separate feature. The minimum fix here is: don't publish `ai.task.completed`.

### Fix C — `dispatchActions` return value

Verify that `dispatchActions` (in `dispatch.go` or `actions.go`) currently returns `(bool, error)` where the bool indicates whether any actions were found. If it only returns `error`, update the signature:

```go
func dispatchActions(ctx context.Context, runID, agencyID, output string) (hasActions bool, err error)
```

Return `(false, nil)` for the "no actions block" case (not an infrastructure error, but a content signal).

## Verification

```bash
# Reproduce: assign MVP-SF-002 (complex 12-file task) to developer-01
# Observe: decomp run status should be AGENT_RUN_STATUS_FAILED (not COMPLETED)
# Observe: ai.task.failed published (not ai.task.completed)
# Observe: task status should be TASK_STATUS_FAILED (not TASK_STATUS_COMPLETED)
# Observe: codevaldai logs: "WARN: run=... dispatchActions: no actions block; publishing ai.run.failed"

# Also verify max_tokens guard:
docker exec codevald-local-codevaldai-1 env | grep -i token
# or check agency.json:
python3 -c "
import json
data = json.load(open('/workspaces/CodeVald-AIProject/CodeValdImplementations/Agencies/utility-app-builder/agency.json'))
for a in data.get('agents', []):
    if a.get('name') == 'developer-agent':
        print('max_tokens:', a.get('max_tokens'))
"
```

## Dependencies

- Related to [BUG-09-028](BUG-09-028_actions_parser_drops_complete_block_when_post_think_truncated.md) (parser fallback for truncated post-think block). That bug's Phase 2 fix (max_tokens bump to 16384) addresses Failure A if the container binary is current. This bug addresses the orthogonal Failure B (no-actions → false task completion) which is still present even at higher token limits.
- Fix B has no dependencies — it is a targeted guard in `execute.go`.
