---
status: 📋 Draft (2026-06-02)
owner: CodeValdAI
scope: AgentRun failure events + field contracts for recovery pipelines
source: gap analysis of `/4-QA/agencies/utility-app-builder/09`
---

# AI Run Failure Modes

CodeValdAI owns `AgentRun` — the unit of LLM work. The 09 QA scenario uses
AgentRuns in three roles: **decomposition** (parent task → todos), **todo
implementation** (per-todo runs that emit `git.*` and `work.todo.completed`),
and **diagnostics** (today only `merge-failure-diagnostics`). Each role has
its own failure surface.

This doc catalogues the failure events CodeValdAI emits and the field
contracts that recovery pipelines (per
[FEAT-20260602-005](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md))
must satisfy when they synthesize AI success events.

The orchestration overview lives in
[CodeValdCross — pipeline-failure-handling](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md).

---

## Failure events CodeValdAI emits

| Event | When emitted | `payload` fields |
|---|---|---|
| `ai.run.failed` | An AgentRun terminates with `status=failed`. Covers all three roles (decomp, todo, diagnostics). | `run_id`, `agent_id`, `task_id`, `todo_id` (if todo run), `workflow_run_id`, `reason` ∈ {`provider_error`, `malformed_output`, `timeout`, `internal_error`, `cancelled`} |
| `ai.run.permanently_failed` | Retry budget exhausted at the AI layer. Today: not produced. Future: emitted after the recovery pipeline's last attempt fails. | `run_id`, `task_id`, `workflow_run_id`, `attempt_count`, `final_reason` |

The reason taxonomy matters: recovery pipelines branch on `reason`. A
`provider_error` may be transient (retry). A `malformed_output` is a prompt
bug (regenerate with a hardened prompt). A `timeout` is ambiguous (retry or
abort, depending on budget). The current code logs free-form error strings;
this needs to be normalized into the typed reasons above.

---

## Field contracts for synthesized success events

### `ai.run.completed` (decomposition role)

Listened for by: nothing directly — but the run's `output` is parsed by
CodeValdWork's `ai.todo.created` bridge to create `TaskTodo` entities. So
the synthesized event must produce a parseable output.

- **Must produce:** `run_id`, `task_id`, `workflow_run_id`, `output`
  containing a valid `\`\`\`actions ... \`\`\`` fence with an
  `ai.todo.created` action carrying the todo list.
- **May differ:** `started_at`, `duration_ms`, `agent_id` (different fix
  agent), `model`, `token_usage`.

A recovery pipeline (`decomp-solving-problem`) that synthesizes this event
**must** emit the actions block in the same fence format the
`parseTodosFromOutput` parser expects. Field contract for the embedded
`ai.todo.created` payload is documented under
[task-decomposition.md](task-decomposition.md).

### `ai.run.completed` (todo implementation role)

Listened for by: nothing directly — todo completion is signalled to
CodeValdWork via `work.todo.completed`, which CodeValdAI emits when the
todo's AgentRun terminates with status=completed.

- **Must produce:** `run_id`, `todo_id`, `task_id`, `workflow_run_id`,
  `output` (may be empty for actions-only todos).
- **May differ:** all other fields.

A recovery pipeline that produces this event for a todo (`impl-solving-problem`)
**must** also emit `work.todo.completed` with the matching `todo_id`. Otherwise
CodeValdWork's `maybeCompleteParentTask` will never see the todo as terminal.

This dual-emission is awkward and is a candidate for simplification:
`work.todo.completed` should be produced by CodeValdWork from observation of
the AI run, not emitted by CodeValdAI directly. Tracked as a follow-up.

### `ai.run.completed` (diagnostics role)

Listened for by: nothing structural — diagnostics output is read by humans
or by an escalation pipeline. No field contract beyond `run_id`,
`workflow_run_id`, `output`.

---

## AR-N — AI-specific failure modes (and which recovery pipeline owns them)

### AR-1 — LLM provider 5xx / network failure (provider_error)

**Trigger:** `huggingface-novita` returns 5xx; gRPC unreachable; rate-limit
429. Today: the run is marked `failed`; nothing downstream reacts.

**Recovery:** `decomp-solving-problem` or `impl-solving-problem` — runs a
retry with exponential backoff (in-pipeline; not via Cross). After N
retries fails terminally → triggers its own failure pipeline (default
`default-failure-pipeline` → `work.run.failed`).

### AR-2 — Malformed actions block (malformed_output)

**Trigger:** LLM returns an `output` with no `\`\`\`actions` fence, or with
invalid JSON inside it. Today: silent — `dispatchActions` logs and no
todos are dispatched.

**Recovery:** `decomp-solving-problem` runs the same agent with a
hardened prompt that includes the previous malformed output as
counter-example: "your previous attempt produced this; do not repeat it."
On success, synthesizes `ai.run.completed` with the corrected output.

This was a real bug ([BUG-09-002](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-compile-pipeline-findings.md))
— the previous fix was to rewrite the prompt. The recovery pipeline makes
this self-healing instead of requiring a manual prompt edit.

### AR-3 — Run timeout (timeout)

**Trigger:** an AgentRun stays in `running` for >N minutes without
publishing a terminal event. Today: no detection. The Cross watchdog
([FEAT-20260602-006](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-006_workflow_run_watchdog.md))
would emit `work.run.timeout` for the WorkflowRun, but not for the
AgentRun itself.

**Recovery:** add per-AgentRun timeout config (default 5 min for impl, 3
min for decomp). On timeout, AI publishes `ai.run.failed { reason:
timeout }` → recovery pipeline fires.

### AR-4 — Cascading diagnostics failure

**Trigger:** `merge-failure-diagnostics` runs and the diagnostics AI itself
fails. Today: silent.

**Post FEAT-005:** the diagnostics AI's plan declares its own
`on_failure_pipeline = default-failure-pipeline` so the failure terminates
the parent run cleanly. Diagnostics-of-diagnostics is bounded by the
nesting cap (default 2).

### AR-5 — AgentRun cancelled mid-flight

**Trigger:** TF-6 cancellation (operator) → CodeValdWork emits
`work.task.cancelled` → CodeValdAI subscribes; flips in-flight AgentRuns to
`cancelled`; publishes `ai.run.failed { reason: cancelled }`.

**Recovery:** no recovery — cancelled runs do not trigger a failure pipeline
(the cancellation path is its own terminal flow). FEAT-005's Cross dispatcher
inspects `reason == cancelled` and skips the failure-pipeline lookup.

### AR-6 — Agent deleted / renamed mid-run

**Trigger:** rare but possible — config reimport while a run is in flight.
The AgentRun's `agent_id` no longer resolves. Today: the run completes
anyway (agent_id is just a reference); only the dispatch path validates
agent existence.

**Fix:** prohibit agent delete while any non-terminal AgentRun references
the agent (return 409). Same rule as TF-7 in
[task-failure-modes](../../../../CodeValdWork/documentation/3-SofwareDevelopment/mvp-details/task-failure-modes.md).

---

## What the recovery pipelines look like

`decomp-solving-problem` (sketch):

```json
{
  "code":             "decomp-solving-problem",
  "trigger_topic":    "work.pipeline.requested",
  "payload_condition": "\"pipeline_code\":\"decomp-solving-problem\"",
  "handler_service":  "codevaldai",
  "agent_id":         "developer-agent",
  "instructions":     "The previous decomposition run for task {task_id} failed with reason: {failed_event.payload.reason}. The original task is: {task.title} — {task.description}. The previous attempt output (if any): {failed_event.payload.last_output}. Produce a corrected decomposition as a proper `actions` fence block with `ai.todo.created` containing the todo list.",
  "success_event":    "ai.run.completed",
  "failure_event":    "ai.run.failed",
  "on_failure_pipeline": "default-failure-pipeline"
}
```

`impl-solving-problem` (sketch):

```json
{
  "code":             "impl-solving-problem",
  "trigger_topic":    "work.pipeline.requested",
  "payload_condition": "\"pipeline_code\":\"impl-solving-problem\"",
  "handler_service":  "codevaldai",
  "agent_id":         "developer-agent",
  "instructions":     "The todo {todo_id} for task {task_id} failed (reason: {failed_event.payload.reason}). The original todo instructions: {todo.instructions}. The failed run's output: {failed_event.payload.output}. Produce the same outputs the todo would have produced — git.file.write events for any code changes, work.todo.completed at the end.",
  "success_event":    "ai.run.completed",
  "failure_event":    "ai.run.failed",
  "on_failure_pipeline": "default-failure-pipeline"
}
```

These are not built today; they ship with FEAT-005.

---

## Open follow-ups

- Normalize `reason` strings into a typed enum in proto. Today error
  reasons are free-form strings — recovery pipelines can't reliably branch
  on them.
- Per-AgentRun timeout config (today none).
- Decision: emit `work.todo.completed` from CodeValdWork instead of
  CodeValdAI, so recovery pipelines only have one event to synthesize.
- Decision: do recovery pipelines that emit `git.file.write` events need to
  produce them on the *same* feature branch, or may they re-create the
  branch? Recommend: same branch (use the failed run's `branch_name`).

---

## Related work

- [Cross — pipeline-failure-handling](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md)
- [Cross — FEAT-20260602-005 — failure pipelines via synthesized success events](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md)
- [Work — task-failure-modes](../../../../CodeValdWork/documentation/3-SofwareDevelopment/mvp-details/task-failure-modes.md)
- [task-decomposition.md](task-decomposition.md) — actions-block format the decomp recovery must reproduce
- [run-execution.md](run-execution.md) — AgentRun lifecycle today
- [BUG-09-002 / BUG-09-003 — malformed actions block](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-compile-pipeline-findings.md)
