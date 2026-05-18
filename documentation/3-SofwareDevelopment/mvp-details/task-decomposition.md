# CodeValdAI — Task Decomposition & `ai.task.todo`

> Part of the mvp-details. Index: [README.md](README.md)
>
> Full bridge implementation (CodeValdWork side) →
> [CodeValdWork: task-decomposition.md](../../../../CodeValdWork/documentation/3-SofwareDevelopment/mvp-details/task-decomposition.md)

---

## Overview

Task decomposition is the mechanism by which a developer agent splits a non-trivial
task into atomic sub-tasks before any implementation work begins.

**CodeValdAI's responsibility is narrow:**
1. Inject the decomposition preamble into the system prompt for task-driven runs.
2. Emit `ai.task.todo` via the standard actions block when the LLM decides to decompose.
3. Publish that event to Cross — nothing else.

**CodeValdWork owns the rest:** it consumes `ai.task.todo`, creates one `TaskTodo` entity
per item, assigns each to the parent task's agent, and publishes `work.task.todo`.
CodeValdAI agents then pick up `work.task.todo` events via work plans and execute each
sub-task as a normal `AgentRun` with `task_id = TodoID`.

> `spawnTodoRuns` has been **removed**. CodeValdAI never spawns child runs internally
> from an `ai.task.todo` payload.

---

## 1. Architecture

### Full Lifecycle

```
work.task.assigned  (ParentTaskID absent)
        │
        ▼ CodeValdAI
RACIDispatcher → triggerPlanRun
        │
        ├── IntakeRun  → pending_intake
        └── ExecuteRunStreaming (decomposition run)
                ├── system prompt = decompositionPreamble + agent.SystemPrompt
                ├── → running; publish "ai.task.in_progress"
                ├── callLLM → LLM outputs ONLY an ```actions block
                │             with topic "ai.task.todo"
                ├── dispatchActions(agentID, output)
                │       └── Publish "ai.task.todo" to Cross
                │           ← CodeValdWork takes it from here
                ├── → completed
                ├── Publish "ai.task.completed"
                └── Publish "ai.run.completed"

─── CodeValdWork bridge (separate service) ───────────────────────────────────
        Receives "ai.task.todo"
        Creates TaskTodo entities; publishes "work.task.todo" × N

─── CodeValdAI: work plan subscribed to "work.task.todo" ─────────────────────
        RACIDispatcher → triggerPlanRun
        AgentRun (task_id = TodoID, NOT the parent Task ID)
        pending_intake → running → completed | failed
        Publishes ai.task.in_progress / ai.task.completed / ai.task.failed
        (CodeValdWork updates TaskTodo.status on receipt)
```

### Interface Boundary

`dispatchActions` (`execute.go`) publishes every action topic to Cross, including
`ai.task.todo`. There is no special-case handling for `ai.task.todo` — the internal
`spawnTodoRuns` path has been removed.

### Graph Entities Touched (CodeValdAI side)

For the **decomposition run**: same `AgentRun` + `RunField` entities as any other run.
The run has `task_id = <parent Work Task ID>`.

For each **todo execution run** (triggered by `work.task.todo`):
- `AgentRun` with `task_id = <TaskTodo ID>` (not the parent Task ID).
- Standard `RunField` / `RunInput` entities.

### Failure Modes

| Failure | Consequence |
|---|---|
| LLM outputs malformed `ai.task.todo` JSON | `dispatchActions` logs; parent run reaches `completed`; `ai.task.todo` event not published; no todos created |
| `ai.task.todo` publish to Cross fails | Logged and swallowed; CodeValdWork never receives it; no todos spawned |
| Todo execution run LLM error | That run → `failed`; `ai.task.failed` published (CodeValdWork updates `TaskTodo.status → failed`); sibling todos unaffected |

---

## 2. Decomposition Preamble (`planner.go`)

### Injection Point

The preamble is prepended to the system prompt in `ExecuteRunStreaming` when:
- `run.TaskID != ""` (task-driven run, not a manually triggered one)
- `segmentNumber == 1` (first session only — not replayed on yield-chain successors)

Child runs triggered by `work.task.todo` have `task_id = TodoID`. The preamble is
still injected on their first session. The LLM will typically skip decomposition
because the `Instructions` field already contains a focused sub-task prompt, but
the guard in the preamble ("Do NOT decompose only when single atomic operation")
is what makes that decision.

### Preamble Template

```
DECOMPOSITION ASSESSMENT — evaluate before acting

Decompose when ANY of the following apply (default to YES for feature work):
  • The task involves implementing code changes
  • The task spans multiple independent concerns
  • There are clearly separable steps
  • Completing it end-to-end would require more than one focused action

Do NOT decompose only when the task is a single atomic operation.

── IF DECOMPOSING ──────────────────────────────────────────────────────────────
Your ENTIRE response must be exactly one actions block — no other text:

```actions
[{"topic":"ai.task.todo","payload":{"parent_task_id":"PARENT_TASK_ID","run_id":"RUN_ID","agent_id":"AGENT_ID","todos":[
  {"title":"…","description":"…","instructions":"…","ordinality":1,"can_run_parallel":true},
  {"title":"…","description":"…","instructions":"…","ordinality":2,"can_run_parallel":false,"depends_on":[1]}
]}}]
```

── IF NOT DECOMPOSING ──────────────────────────────────────────────────────────
Ignore the section above entirely and proceed with your normal execution actions.
```

`buildDecompositionPreamble(taskID, runID, agentID string)` substitutes the run-specific
IDs into the template before injection.

---

## 3. Data Models (`events.go`)

```go
// TopicTaskTodo is the ai.* domain event published when a developer agent
// decomposes a task. CodeValdWork is the sole consumer; it creates TaskTodo
// entities and publishes work.task.todo per item.
const TopicTaskTodo = "ai.task.todo"

// TaskTodoPayload is the payload for ai.task.todo.
type TaskTodoPayload struct {
    ParentTaskID string     `json:"parent_task_id"` // originating Work Task ID
    RunID        string     `json:"run_id"`
    AgentID      string     `json:"agent_id"`
    Todos        []TodoItem `json:"todos"`
}

// TodoItem describes one sub-task within a TaskTodoPayload.
type TodoItem struct {
    Title          string `json:"title"`
    Description    string `json:"description"`
    Instructions   string `json:"instructions"` // full self-contained agent prompt
    Ordinality     int    `json:"ordinality"`   // 1-based
    CanRunParallel bool   `json:"can_run_parallel"`
    DependsOn      []int  `json:"depends_on,omitempty"`
}
```

---

## 4. Registrar Topics (`internal/registrar/registrar.go`)

`"ai.task.todo"` is in the **produces** list. CodeValdWork declares it in its
**consumes** list — the Cross subscription wires them together.

```go
[]string{ // produces
    "ai.agent.created",
    "ai.run.completed",
    "ai.run.failed",
    "ai.task.in_progress",
    "ai.task.completed",
    "ai.task.failed",
    "ai.task.yielded",
    "ai.task.todo",
}
```

---

## 5. Work Plan Instruction Pattern

### Decompose-First Guard

Work plan instructions for developer agents include a detection step so the LLM
knows whether it is handling an original task (decompose) or a todo execution run
(implement directly).

The `work.task.todo` payload carries `ParentTaskID` — if the agent checks this field
in the event payload and it is non-empty, it is executing a todo and should implement.

```
STEP 1 — DETECT CONTEXT
Check the event payload for a "ParentTaskID" field.
- ParentTaskID IS present and non-empty → skip to STEP 3 (implement directly).
- ParentTaskID is ABSENT or empty → continue to STEP 2 (decompose).

STEP 2 — DECOMPOSE (original tasks only)
Assess the task. For feature work: always decompose into 2–5 atomic sub-tasks.
Output ONLY the ai.task.todo actions block — no prose.

STEP 3 — IMPLEMENT (todo execution runs)
Execute the instructions exactly. Output a single ```actions block.
```

---

## 6. AgentRun.task_id for Todo Execution Runs

When a CodeValdAI agent picks up a `work.task.todo` event, the resulting `AgentRun`
has `task_id = TodoID` (the `TaskTodo` entity ID in CodeValdWork) — **not** the
parent `Task` ID.

This means:
- `ai.task.in_progress / ai.task.completed / ai.task.failed` reference the `TodoID`.
- CodeValdWork can update `TaskTodo.status` without ambiguity.
- The parent Task ID is available via `payload.ParentTaskID` in the dispatch
  instructions if the agent needs to reference it.

---

## 7. Acceptance Tests

| Test | Expected |
|---|---|
| Developer receives task with no `ParentTaskID` | Decomposition preamble injected; LLM output contains `ai.task.todo` actions block |
| `dispatchActions` on `ai.task.todo` | Event published to Cross; no internal child run spawned |
| Developer receives `work.task.todo` with `ParentTaskID` set | No decomposition; LLM outputs implementation actions |
| `ai.task.todo` payload malformed | `dispatchActions` logs error; parent run still `completed`; event not published |
| Todo execution run completes | `AgentRun.task_id == TodoID`; `ai.task.completed` references `TodoID` |
| Todo execution run fails | `ai.task.failed` references `TodoID`; sibling runs unaffected |
| Publisher nil | `ai.task.todo` publish silently skipped; no panic |
