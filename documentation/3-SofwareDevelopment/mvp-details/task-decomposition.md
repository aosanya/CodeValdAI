# CodeValdAI — Task Decomposition & `ai.task.todo`

> Part of the mvp-details. Index: [README.md](README.md)

---

## Overview

Task decomposition is an **AI-internal** pattern. When a developer agent receives a new
(non-sub-task) `work.task.assigned` event, it splits the task into atomic sub-tasks and
emits an `ai.task.todo` actions block. CodeValdAI intercepts that action in
`dispatchActions` and spawns a child `AgentRun` for each `TodoItem` directly — no
CodeValdWork involvement required.

Sub-tasks are pure `AgentRun` entities. They have no corresponding CodeValdWork task and
carry no `TaskID`. Their lifecycle is tracked entirely within CodeValdAI.

---

## 1. Architecture & Design

### Full Lifecycle

```
work.task.assigned  (ParentTaskID absent)
    │
    ▼
RACIDispatcher → triggerPlanRun
    │
    ├── IntakeRun  → pending_intake
    └── ExecuteRunStreaming (decomposition run)
            ├── → running
            ├── Publish "ai.task.in_progress"
            ├── callLLM → LLM outputs ONLY an ```actions block
            │             with topic "ai.task.todo"
            ├── dispatchActions(agentID, output)
            │       ├── Publish "ai.task.todo" to Cross  (observability only)
            │       └── spawnTodoRuns(agentID, payload)
            │               └── goroutine × N:
            │                       IntakeRun(instructions=todo.Instructions)
            │                       ExecuteRunStreaming(run.ID, nil)
            ├── → completed
            ├── Publish "ai.task.completed"
            └── Publish "ai.run.completed"

Each child run (N = len(todos)):
    pending_intake → running → completed | failed
    No work.task.* events — purely AI-internal.
```

### Interface Boundary
`ai.task.todo` is handled by the extended `dispatchActions` method in `execute.go`.
No changes to `AIManager` interface. No Cross routing to CodeValdWork.

### Graph Entities Touched
No new entity types. Each child run creates the same `AgentRun` + `RunField` entities
as any other run. Child runs have `TaskID = ""`.

### Failure Modes

| Failure | Consequence |
|---|---|
| LLM outputs malformed `ai.task.todo` JSON | `dispatchActions` logs error; parent run still reaches `completed`; no children spawned |
| `spawnTodoRuns` `IntakeRun` error | Logged per todo; other todos still spawn |
| Child run LLM error | Child run → `failed`; `ai.run.failed` published; other children unaffected |

---

## 2. Run Lifecycle

### Decomposition Run Status Transitions
```
pending_intake → pending_execution → running → completed
```
Identical to any other run. The LLM output for this run is a single `ai.task.todo` actions
block.

### Child Run Status Transitions
```
pending_intake → pending_execution → running → completed | failed
```
Child runs are independent. They use the same agent as the parent decomposition run.

### Sub-task Detection
The `work.task.assigned` payload includes a `ParentTaskID` field when a task was created
as a child of another task. Child runs spawned by `spawnTodoRuns` have no `TaskID`, so
they never produce `work.task.*` events and cannot receive `work.task.assigned`.

Work plan instructions use a simpler guard: if `ParentTaskID` is **absent or empty** in the
event payload, decompose. If present, implement directly. This prevents infinite recursion
when a future integration creates real sub-tasks in CodeValdWork.

---

## 3. Data Models (`events.go`)

```go
// TopicTaskTodo is published (for observability) and handled internally when
// a developer agent decomposes a task into sub-tasks. dispatchActions intercepts
// this topic to spawn child AgentRuns via spawnTodoRuns.
const TopicTaskTodo = "ai.task.todo"

// TaskTodoPayload is the payload for ai.task.todo.
type TaskTodoPayload struct {
    ParentTaskID string     `json:"parent_task_id"` // originating work task ID
    RunID        string     `json:"run_id"`
    AgentID      string     `json:"agent_id"`
    Todos        []TodoItem `json:"todos"`
}

// TodoItem describes one sub-task within a TaskTodoPayload.
// Ordinality is 1-based. DependsOn references ordinality values of
// prerequisites. CanRunParallel and DependsOn are stored for future
// sequential scheduling support; all todos are currently spawned immediately.
type TodoItem struct {
    Title          string `json:"title"`
    Description    string `json:"description"`
    Instructions   string `json:"instructions"`         // full prompt for the child run
    Ordinality     int    `json:"ordinality"`
    CanRunParallel bool   `json:"can_run_parallel"`
    DependsOn      []int  `json:"depends_on,omitempty"`
}
```

---

## 4. Implementation (`execute.go`)

### `dispatchActions` (updated signature)

```go
func (m *aiManager) dispatchActions(ctx context.Context, agentID, output string)
```

For every action in the parsed block:
1. Publish to Cross via `m.publisher` (best-effort, skipped when publisher is nil)
2. If `topic == "ai.task.todo"`: unmarshal payload and call `spawnTodoRuns`

### `spawnTodoRuns`

```go
func (m *aiManager) spawnTodoRuns(agentID string, payload TaskTodoPayload)
```

Spawns one goroutine per `TodoItem`. Each goroutine calls:
```
IntakeRun(agentID, todo.Instructions, TaskID="")
ExecuteRunStreaming(run.ID, nil, noopChunk)
```

**Sequential scheduling** (`depends_on`) is not enforced in this implementation.
All todos are dispatched immediately. The `Instructions` field on each todo must be
self-contained enough that the LLM can act without cross-todo coordination.
Sequential enforcement is deferred — see Future Work.

### `unmarshalActionPayload` helper

```go
func unmarshalActionPayload(a Action, target any) error
```

Round-trips `Action.Payload` (a `map[string]any`) through JSON into a typed struct.
Used exclusively for `ai.task.todo` payload unmarshalling.

### Call site in `ExecuteRunStreaming`

```go
m.dispatchActions(ctx, agent.ID, finalOutput)
```

---

## 5. Registrar (`internal/registrar/registrar.go`)

`"ai.task.todo"` is declared in the **produces** list so Cross can log and route the
event for observability. No service needs to subscribe to it for sub-task spawning to work.

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
},
```

---

## 6. Work Plan Instruction Pattern

### Rule: Decompose-First, Implement-Second

Work plan `instructions` for developer agents must contain the three-step guard below.
The LLM is the only actor that decides decomposition — CodeValdAI enforces nothing at the
Go layer beyond spawning the runs the LLM requests.

```
STEP 1 — DETECT SUB-TASK
Check the event payload for a "ParentTaskID" field.
- If "ParentTaskID" IS present and non-empty: skip to STEP 3.
- If "ParentTaskID" is ABSENT or empty: continue to STEP 2.

STEP 2 — DECOMPOSE (first-time tasks only)
Break the task into 2–5 atomic sub-tasks. Each must be completable in one agent session.

For each sub-task specify:
  title           — short, imperative (≤ 8 words)
  description     — one sentence: what the sub-task achieves
  instructions    — complete implementation prompt for the child agent
  ordinality      — 1-based order number
  can_run_parallel — true if no dependency on other sub-tasks
  depends_on      — [] or list of ordinality numbers that must complete first

Output ONLY this actions block (no prose):
```actions
[{
  "topic": "ai.task.todo",
  "payload": {
    "parent_task_id": "<TaskID from event payload>",
    "run_id": "",
    "todos": [ ... ]
  }
}]
```
A response with no ```actions block is always invalid.

STEP 3 — IMPLEMENT (child runs; ParentTaskID present or instructions are a sub-task prompt)
Implement the feature described. Output a single ```actions block.
```

---

## 7. Parallel vs Sequential Sub-task Scheduling

### Current Behaviour
All todos are spawned immediately in goroutines. `can_run_parallel` and `depends_on` are
stored in the payload but not yet enforced by `spawnTodoRuns`.

### Encoding Intent (for future enforcement)

| `can_run_parallel` | `depends_on` | Intended behaviour |
|---|---|---|
| `true`  | `[]`    | Start immediately alongside other parallel todos |
| `false` | `[1]`   | Start only after todo with ordinality 1 completes |
| `true`  | `[1,2]` | Parallel with others, but only after 1 and 2 complete |

### Future Work
Sequential scheduling requires `spawnTodoRuns` to wait for goroutines whose ordinality
appears in `depends_on`. A simple approach: run todos in ordinality order, use a
`map[int]chan struct{}` to signal completion, and block goroutines that depend on
incomplete predecessors.

---

## 8. Acceptance Tests

| Test | Expected |
|---|---|
| Developer receives task with no `ParentTaskID` | LLM output contains `ai.task.todo` actions block |
| `dispatchActions` intercepts `ai.task.todo` | `spawnTodoRuns` called; N child `IntakeRun` calls |
| Developer receives task with non-empty `ParentTaskID` | No `ai.task.todo`; LLM outputs implementation actions |
| `ai.task.todo` payload malformed | `dispatchActions` logs error; parent run still `completed` |
| N todos in payload | N child runs created and started |
| Child run LLM error | Child run → `failed`; sibling runs unaffected |
| `spawnTodoRuns` with publisher nil | Child runs still spawn; publish step silently skipped |

---

## 9. Example: `08-work-02-ai-run.md` Task Flow

Task: **"Add dark mode toggle to settings screen"**

**Run 1 — Decomposition** (triggered by test Work-2, `$ASSIGN_TIME`)
1. `work.task.assigned` arrives; no `ParentTaskID`
2. Developer agent outputs:
   ```actions
   [{"topic":"ai.task.todo","payload":{"parent_task_id":"<NEW_TASK_ID>","todos":[...]}}]
   ```
3. `dispatchActions` publishes `ai.task.todo` to Cross (observability)
4. `spawnTodoRuns` spawns 4 child `AgentRun`s in goroutines
5. Run 1 status: `completed`

**Runs 2–5 — Implementation** (child runs, no `TaskID`)
- Each child run executes its `TodoItem.Instructions` independently
- Status per run: `completed` or `failed`

**Test assertion**: The run created after `$ASSIGN_TIME` reaches `completed`. Its output
contains an `ai.task.todo` actions block rather than `git.branch.create`. Additional runs
without a `TaskID` appear in the run list as the child runs execute.
