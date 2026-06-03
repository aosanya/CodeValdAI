# BUG-20260603-003 (AI) — Todo returning empty actions block `[]` is marked AGENT_RUN_STATUS_FAILED

**Status:** 📋 Open
**Severity:** High — a valid "no-op" todo outcome causes AGENT_RUN_STATUS_FAILED, which cascades to TASK_STATUS_FAILED and WORKFLOW_RUN_STATUS_FAILED
**Owner:** CodeValdAI
**Estimated effort:** S — add a check in the AI run result handler: treat `actions = []` as a success (no-op), not an error
**Source finding:** QA scenario 09 run 2026-06-03 — todo "Compile and verify branch" returned `[]` as its actions block; AI dispatch marked the run FAILED; task cascaded to FAILED; WorkflowRun cascaded to FAILED

## Problem

The agent decomposition instructions explicitly allow a todo to emit `[]` (an empty actions array) when the work it would do is already complete (e.g. a "compile and verify" todo that runs after all files are written finds the build already clean). This is documented as a valid sentinel in the agent prompt:

> "If context contains 'Task Branch: <name>', respond with [] and stop immediately. The branch already exists; do not re-decompose."

The AI dispatch layer (`CodeValdAI`) interprets `actions = []` as a failure rather than a success: the run is recorded as `AGENT_RUN_STATUS_FAILED`, which triggers the `ai.task.failed` event, which Work handles by calling `FailTodo` on the parent todo, which cascades to `TASK_STATUS_FAILED` via `maybeCompleteParentTask`.

Consequence: any pipeline run that produces a no-op todo (which is expected and correct behaviour) terminates the entire task with a failure status. The WorkflowRun UI shows the run as FAILED despite the actual work completing successfully.

Observed in two consecutive QA runs (2026-06-03): each produced 2 AGENT_RUN_STATUS_FAILED entries out of ~17, both from todos designed to emit `[]`.

## Evidence

```
AI runs for workflow run c4821356 (2026-06-03T09:05–09:26):
  id=a56fe6eb  status=AGENT_RUN_STATUS_FAILED  created=2026-06-03T09:26:18Z
  id=13fa78ec  status=AGENT_RUN_STATUS_FAILED  created=2026-06-03T09:15:44Z

  Out of 17 total runs: 15 COMPLETED, 2 FAILED

Final task state:
  MVP-SF-001  status=TASK_STATUS_FAILED  wrid=c4821356-...

WorkflowRun state:
  c4821356  status=WORKFLOW_RUN_STATUS_FAILED
```

The failed todos (ordinality: the "compile and verify" step, last in the list) correspond to the agent instruction RULE IDEMPOTENT: if the branch already exists, emit `[]`. The branch existed; the agent complied; the result was marked FAILED.

## Root cause

In `CodeValdAI` the AI run result handler checks the returned actions payload. When `actions` is an empty slice `[]`, the handler treats it as a missing or malformed result and marks the run as FAILED. The correct behaviour is:

- `actions = []` → no-op success → `AGENT_RUN_STATUS_COMPLETED`, no `ai.task.failed` published
- `actions` is absent / null / malformed → `AGENT_RUN_STATUS_FAILED`

The distinction between "intentionally empty" and "erroneously empty" is already established in the agent instructions, but the dispatcher does not honour it.

## Fix plan

In the AI run execution/result-handling code (likely `execute.go` or the AI run finaliser), change the post-run status decision:

```go
// Before (wrong):
if len(actions) == 0 {
    markFailed("no actions returned")
}

// After (correct):
if actions == nil {  // nil = missing/unparseable → real failure
    markFailed("actions block missing or unparseable")
} else {             // empty slice = explicit no-op → success
    markCompleted()
}
```

This requires distinguishing between a nil slice (JSON `null` or absent key) and an empty slice (JSON `[]`). Both Go and the JSON decoder support this distinction natively.

No proto changes needed. The fix is contained to the AI run result handler in CodeValdAI.

## Verification

1. Craft a todo whose agent response returns `{"actions": []}`.
2. Confirm: the AI run is recorded as `AGENT_RUN_STATUS_COMPLETED`.
3. Confirm: no `ai.task.failed` event is published.
4. Confirm: the parent task does not cascade to FAILED.
5. Run QA scenario 09 end-to-end: WorkflowRun should reach COMPLETED (or FAILED only if a real failure occurs).

## Dependencies

None. Isolated to CodeValdAI result handling.

Related: [BUG-20260603-001 (AI)](BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md) — the inverse case: a decomposition run with no actions block falsely completes the task. That bug and this one both stem from insufficient handling of the `actions` field edge cases.
