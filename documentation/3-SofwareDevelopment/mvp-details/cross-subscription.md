# CodeValdAI — Cross Subscription & Event Receipt

## Overview

CodeValdAI subscribes to `work.task.status.changed` so it can react when a
task goes `pending → in_progress`. Everything flows via Cross — CodeValdAI
never dials PubSub directly.

---

## 1. Confirmed Design Decisions

| Decision | Choice |
|---|---|
| Subscribe registration | Cross handles it — AI declares `consumes` in heartbeat; Cross calls `PubSub.Subscribe` on its behalf |
| Push delivery | Cross calls `EventReceiverService.NotifyEvent` on CodeValdAI |
| Write order | AI writes `ReceivedEvent` to `ai_received_events` **first**, then returns success |
| On DB write failure | Return gRPC error to Cross — delivery stays `pending` |
| Who calls `Ack` | Cross, immediately after a successful `NotifyEvent` response |
| `NotifyEvent` proto | Shared `EventReceiverService` from SharedLib — not defined in `ai.proto` |
| New config | None — uses existing `CROSS_GRPC_ADDR` connection |

---

## 2. Declaring Intent — `consumes` in Registrar

In `internal/registrar/registrar.go`, add `"work.task.status.changed"` to the
`consumes` list passed to `sharedregistrar.New`:

```go
hb, err := sharedregistrar.New(
    crossAddr, advertiseAddr, agencyID,
    "codevaldai",
    []string{  // produces
        "cross.ai.{agencyID}.agent.created",
        "cross.ai.{agencyID}.run.completed",
        "cross.ai.{agencyID}.run.failed",
    },
    []string{  // consumes ← NEW
        "work.task.status.changed",
    },
    routes, pingInterval, pingTimeout,
)
```

On every heartbeat, Cross reads this list and calls `PubSub.Subscribe` on
CodeValdAI's behalf. PubSub is idempotent on `(subscriber_service, topic_pattern)`,
so repeat calls are safe.

---

## 3. Schema Addition

Add `ReceivedEvent` to `DefaultAISchema()` in `schema.go`:

```go
import "github.com/aosanya/CodeValdSharedLib/eventreceiver"

func DefaultAISchema() types.Schema {
    return types.Schema{
        ID:      "ai-schema-v1",
        Version: 1,
        Tag:     "v1",
        Types:   append(aiTypes(), eventreceiver.ReceivedEventTypeDefinition("ai")),
    }
}
```

This seeds the `ai_received_events` ArangoDB collection on startup (idempotent).

### `ReceivedEvent` fields

| Field | Type | Description |
|---|---|---|
| `event_id` | string | PubSub event ID |
| `topic` | string | e.g. `work.task.status.changed` |
| `agency_id` | string | Owning agency |
| `source` | string | Originating service, e.g. `codevaldwork` |
| `payload` | string | Raw JSON from the publisher |
| `received_at` | string | RFC3339 UTC timestamp of receipt |

No `status` field for MVP — pure log.

---

## 4. `EventReceiverService` gRPC Registration

Register the SharedLib `EventReceiverService` alongside the existing `AIService`
in `internal/app/app.go`:

```go
import (
    sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
)

sharedev1.RegisterEventReceiverServiceServer(grpcServer, server.NewEventReceiver(backend, cfg.AgencyID))
```

The fully-qualified gRPC path Cross calls:

```
/codevaldshared.v1.EventReceiverService/NotifyEvent
```

---

## 5. `NotifyEvent` Handler

Create `internal/server/event_receiver.go`:

```go
// EventReceiverServer implements sharedev1.EventReceiverServiceServer.
type EventReceiverServer struct {
    sharedev1.UnimplementedEventReceiverServiceServer
    backend  entitygraph.DataManager
    agencyID string
}

func NewEventReceiver(backend entitygraph.DataManager, agencyID string) *EventReceiverServer {
    return &EventReceiverServer{backend: backend, agencyID: agencyID}
}

// NotifyEvent receives a pushed event from Cross.
// Writes a ReceivedEvent record FIRST; returns error if the write fails so
// Cross leaves the delivery in "pending" state.
func (s *EventReceiverServer) NotifyEvent(ctx context.Context, req *sharedev1.NotifyEventRequest) (*sharedev1.NotifyEventResponse, error) {
    _, err := s.backend.CreateEntity(ctx, s.agencyID, entitygraph.CreateEntityRequest{
        TypeID: "ReceivedEvent",
        Properties: map[string]any{
            "event_id":    req.GetEventId(),
            "topic":       req.GetTopic(),
            "agency_id":   req.GetAgencyId(),
            "source":      req.GetSource(),
            "payload":     req.GetPayload(),
            "received_at": time.Now().UTC().Format(time.RFC3339),
        },
    })
    if err != nil {
        log.Printf("codevaldai: NotifyEvent: write ReceivedEvent: %v", err)
        return nil, status.Errorf(codes.Internal, "log received event: %v", err)
    }
    log.Printf("codevaldai: NotifyEvent: ACK event_id=%s topic=%s source=%s",
        req.GetEventId(), req.GetTopic(), req.GetSource())
    return &sharedev1.NotifyEventResponse{}, nil
}
```

---

## 6. `work.task.status.changed` Payload Shape

Published by CodeValdWork. JSON-encoded in `NotifyEventRequest.payload`:

```json
{
  "task_id": "4ac83b8b-a42b-4e3a-a308-519e3a1bcdae",
  "from":    "pending",
  "to":      "in_progress"
}
```

For MVP, the payload is stored verbatim in `ReceivedEvent.payload`. Future
iterations will parse it and trigger an `AgentRun` when `to == "in_progress"`.

---

## 7. Full Sequence (Happy Path)

```
CodeValdWork → Cross.Publish("work.task.status.changed", payload)
    │
    └── Cross → CodeValdAI.EventReceiverService/NotifyEvent(event_id, topic, payload)
                    │
                    ├── write ReceivedEvent to ai_received_events ✓
                    ├── log: "ACK event_id=... topic=work.task.status.changed"
                    └── return NotifyEventResponse{}
                            │
                            └── Cross → PubSub.Ack(subscriptionID, eventID)
                                            └── Delivery.status → "acked"
```

On DB write failure:

```
NotifyEvent returns gRPC Internal error
    └── Cross logs error, does nothing
            └── Delivery stays "pending" (retry mechanism picks up later)
```

---

## 8. Definition of Done

- [ ] `"work.task.status.changed"` added to `consumes` in `internal/registrar/registrar.go`
- [ ] `eventreceiver.ReceivedEventTypeDefinition("ai")` added to `DefaultAISchema()`
- [ ] `ai_received_events` collection seeded idempotently on startup
- [ ] `EventReceiverServiceServer` registered on gRPC server in `internal/app/app.go`
- [ ] `internal/server/event_receiver.go` created with `NotifyEvent` handler
- [ ] Handler writes `ReceivedEvent` first; returns `codes.Internal` on failure
- [ ] Handler logs `ACK event_id=... topic=... source=...` on success
- [ ] Unit test: success path writes entity and returns success
- [ ] Unit test: DB failure returns gRPC Internal error
- [ ] SharedLib dependency (`SHAREDLIB-018`) must be complete before this task
