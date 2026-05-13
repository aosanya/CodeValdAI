# CodeValdAI — Cross Subscription & Event Receipt

## Overview

CodeValdAI subscribes to `work.task.assigned` (and related `work.task.*` topics)
so it can react when a task is assigned to an AI agent. Everything flows via Cross
— CodeValdAI never dials PubSub directly.

**Domain rule:** CodeValdAI only publishes `ai.*` events. It never publishes into
`work.*`, `git.*`, or `comm.*`. The task status feedback loop is handled by a bridge:
CodeValdAI publishes `ai.task.*` → CodeValdWork consumes and publishes `work.task.*`.

---

## 1. Confirmed Design Decisions

| Decision | Choice |
|---|---|
| Subscribe registration | Cross handles it — AI declares `consumes` in heartbeat; Cross calls `PubSub.Subscribe` on its behalf |
| Push delivery | Cross calls `EventReceiverService.NotifyEvent` on CodeValdAI |
| Write order | AI writes `ReceivedEvent` to `ai_received_events` **first**, then dispatches |
| On DB write failure | Return gRPC error to Cross — delivery stays `pending` |
| Who calls `Ack` | Cross, immediately after a successful `NotifyEvent` response |
| `NotifyEvent` proto | Shared `EventReceiverService` from SharedLib — not defined in `ai.proto` |
| New config | None — uses existing `CROSS_GRPC_ADDR` connection |

---

## 2. Topics Produced by CodeValdAI

| Topic | Published when | Payload |
|---|---|---|
| `ai.task.in_progress` | `ExecuteRunStreaming` transitions run to `running` (before LLM call) | `{task_id, run_id, agent_id}` |
| `ai.task.completed` | LLM finishes successfully and actions dispatched | `{task_id, run_id, agent_id}` |
| `ai.task.failed` | LLM errors, times out, or produces no actions block | `{task_id, run_id, reason, failed_by{agent_id, work_plan_id, work_plan_code}}` |
| `ai.task.yielded` | Session hits wall-clock or token limit; successor run created | `{task_id, run_id, chain_id, segment_number, tokens_used, partial_output}` |
| `ai.task.todo` | Developer agent decomposes a task; child runs spawned internally by `spawnTodoRuns` | `{parent_task_id, run_id, agent_id, todos[]}` — see [task-decomposition.md](task-decomposition.md) |
| `ai.run.completed` | Same as `ai.task.completed` — for AI-internal consumers | `{run_id}` |
| `ai.run.failed` | Same as `ai.task.failed` — for AI-internal consumers | `""` |
| `ai.agent.created` | New Agent entity created | `agentID` |

---

## 3. Topics Consumed by CodeValdAI

| Topic | Publisher | What CodeValdAI does |
|---|---|---|
| `work.task.assigned` | CodeValdWork | RACIDispatcher matches payload against work plans; triggers AgentRun if `agent_id` set |
| `git.branch.fetched` | CodeValdGit | RACIDispatcher — triggers AI Code Reviewer run |
| `git.branch.merged` | CodeValdGit | RACIDispatcher — triggers AI Documentor run |
| `work.task.status.changed` | CodeValdWork | Logged as ReceivedEvent; no dispatch yet |

---

## 4. Declaring Intent — `consumes` in Registrar

In `internal/registrar/registrar.go`, the `consumes` list passed to `sharedregistrar.New`:

```go
hb, err := sharedregistrar.New(
    crossAddr, advertiseAddr, agencyID,
    "codevaldai",
    []string{  // produces
        "ai.task.in_progress",
        "ai.task.completed",
        "ai.task.failed",
        "ai.agent.created",
        "ai.run.completed",
        "ai.run.failed",
    },
    []string{  // consumes
        "work.task.assigned",
        "git.branch.fetched",
        "git.branch.merged",
        "work.task.status.changed",
    },
    routes, pingInterval, pingTimeout,
)
```

---

## 5. Schema Addition

`ReceivedEvent` is seeded in `DefaultAISchema()` via the SharedLib helper:

```go
func DefaultAISchema() types.Schema {
    return types.Schema{
        ID:      "ai-schema-v1",
        Version: 1,
        Tag:     "v1",
        Types:   append(aiTypes(), eventreceiver.ReceivedEventTypeDefinition("ai")),
    }
}
```

### `ReceivedEvent` fields

| Field | Type | Description |
|---|---|---|
| `event_id` | string | PubSub event ID |
| `topic` | string | e.g. `work.task.assigned` |
| `agency_id` | string | Owning agency |
| `source` | string | Originating service, e.g. `codevaldwork` |
| `payload` | string | Raw JSON from the publisher |
| `received_at` | string | RFC3339 UTC timestamp of receipt |

---

## 6. `EventReceiverService` gRPC Registration

```go
sharedev1.RegisterEventReceiverServiceServer(grpcServer, server.NewEventReceiver(backend, cfg.AgencyID))
```

The fully-qualified gRPC path Cross calls:

```
/codevaldshared.v1.EventReceiverService/NotifyEvent
```

---

## 7. `work.task.assigned` Payload Shape

Published by CodeValdWork. JSON-encoded in `NotifyEventRequest.payload`:

```json
{
  "TaskID":    "4ac83b8b-a42b-4e3a-a308-519e3a1bcdae",
  "AgentID":   "5e367e1b-4c56-407b-849c-98d2481d2fd3",
  "RoleName":  "Developer",
  "TaskCode":  "UTIL-001",
  "Title":     "Add dark mode toggle",
  "Description": "..."
}
```

RACIDispatcher uses `RoleName` (via `payload_condition`) and the plan's `agent_id` to
decide whether to trigger a run. The `TaskID` is carried through the run lifecycle and
included in all `ai.task.*` publish payloads.

---

## 8. Full Sequence (Happy Path — task assigned → run completes)

```
CodeValdWork → Cross.Publish("work.task.assigned", payload)
    │
    └── Cross → CodeValdAI.EventReceiverService/NotifyEvent
                    │
                    ├── write ReceivedEvent to ai_received_events ✓
                    ├── dispatcher.Dispatch() → RACIDispatcher
                    │       ├── MatchWorkPlans(topic, payload)
                    │       └── triggerPlanRun(plan, payload)
                    │               ├── IntakeRun()  → AgentRun{pending_intake}
                    │               └── ExecuteRunStreaming()
                    │                       ├── → running
                    │                       ├── Publish "ai.task.in_progress"
                    │                       ├── callLLM()
                    │                       ├── → completed
                    │                       ├── dispatchActions()
                    │                       ├── Publish "ai.task.completed"
                    │                       └── Publish "ai.run.completed"
                    └── return NotifyEventResponse{}

CodeValdWork (EventReceiver) ←── Cross fans out "ai.task.in_progress"
    └── UpdateTask(status=in_progress) → Publish "work.task.in_progress"

CodeValdWork (EventReceiver) ←── Cross fans out "ai.task.completed"
    └── UpdateTask(status=completed) → Publish "work.task.completed"
```
