# Yielded Sessions — Implementation Details

Covers MVP-AI-021 through MVP-AI-024.

---

## Background — Why Yielded Sessions

A single `ExecuteRunStreaming` call may not be sufficient for long-running tasks. Token budgets
and wall-clock limits mean the LLM can be cut off mid-response. Rather than failing the run,
CodeValdAI captures the partial output, marks the run as `YIELDED`, and starts a successor
session that replays the full conversation history. A chain of sessions shares a `chain_id`;
a per-agent (overridable per work plan) `max_sessions` cap is the hard circuit breaker.

---

## MVP-AI-021 — Schema & Domain Model Additions

**Status**: 🔲 Not Started
**Branch**: `feature/AI-021_yielded_schema`
**Depends on**: MVP-AI-018 (streaming RPC)

### New fields on `AgentRun`

| Field | Type | Description |
|-------|------|-------------|
| `chain_id` | string (UUID) | Shared across all sessions in a chain. Set on session 1; copied to every successor. Empty on runs not part of a chain. |
| `segment_number` | int | 1-based position in the chain. Session 1 = `1`. O(1) budget check: `segment_number >= max_sessions`. |
| `partial_output` | string | Text streamed before the limit was hit. Stored at yield time; also included in `ai.task.yielded` payload. |

`AgentRun` already has `output` (final), `input_tokens`, `output_tokens`. The new `partial_output`
field is only populated on `YIELDED` runs; `output` remains empty for those.

### New `AgentRunStatus`

```go
// models.go
const (
    // existing
    AgentRunStatusPendingIntake    AgentRunStatus = "pending_intake"
    AgentRunStatusPendingExecution AgentRunStatus = "pending_execution"
    AgentRunStatusRunning          AgentRunStatus = "running"
    AgentRunStatusCompleted        AgentRunStatus = "completed"
    AgentRunStatusFailed           AgentRunStatus = "failed"

    // new
    AgentRunStatusYielded AgentRunStatus = "yielded"
    // Yielded: the run hit a wall-clock or token limit before producing a final
    // result. Partial output is stored; a successor run continues in the same chain.
)
```

### New fields on `Agent`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `session_max_seconds` | int | 300 (5 min) | Wall-clock limit per session. 0 = no limit. |
| `session_max_tokens` | int | 0 | Output token budget per session. 0 = no limit. Note: distinct from `max_tokens` which is the LLM generation limit passed in the request. |
| `session_max_sessions` | int | 1 | Max sessions in a chain. 1 = no yielding (current behaviour). |

### New override fields on `WorkPlan`

Same three fields (`session_max_seconds`, `session_max_tokens`, `session_max_sessions`). A zero value means
"use the agent default"; a non-zero value overrides it. Resolved at dispatch time by
`resolveSessionLimits(agent, workPlan)`.

### Schema additions (`schema.go`)

```go
// AgentRun TypeDef additions
"chain_id":       {Type: "string"},
"segment_number": {Type: "int"},
"partial_output": {Type: "string"},

// Agent TypeDef additions
"session_max_seconds":  {Type: "int"},
"session_max_tokens":   {Type: "int"},
"session_max_sessions": {Type: "int"},

// WorkPlan TypeDef additions (overrides)
"session_max_seconds":  {Type: "int"},
"session_max_tokens":   {Type: "int"},
"session_max_sessions": {Type: "int"},
```

### Proto additions (`ai.proto`)

```proto
// AgentRunStatus enum — add:
AGENT_RUN_STATUS_YIELDED = 6;

// AgentRun message — add:
string chain_id       = 12;
int32  segment_number = 13;
string partial_output = 14;

// Agent message — add:
int32 session_max_seconds  = 10;
int32 session_max_tokens   = 11;
int32 session_max_sessions = 12;
```

### Acceptance Tests

| Test | Expected |
|------|----------|
| `AGENT_RUN_STATUS_YIELDED` maps to gRPC enum value 6 | ✅ |
| `AgentRun` with `chain_id` + `segment_number` round-trips through ArangoDB | ✅ |
| `Agent.max_sessions = 0` is treated as 1 (no yield) | ✅ |

---

## MVP-AI-022 — Yielded Execution Engine

**Status**: 🔲 Not Started
**Branch**: `feature/AI-022_yielded_execution`
**Depends on**: MVP-AI-021

### Goal

Modify `ExecuteRunStreaming` to enforce per-session limits and execute the yield path when
either limit fires.

### Limit Resolution

```go
type SessionLimits struct {
    MaxSeconds  int // 0 = no wall-clock limit
    MaxTokens   int // 0 = no token limit
    MaxSessions int // minimum 1
}

// resolveSessionLimits merges agent defaults with work plan overrides.
// A non-zero work plan field wins over the agent field.
func resolveSessionLimits(agent Agent, plan WorkPlan) SessionLimits {
    limits := SessionLimits{
        MaxSeconds:  coalesce(plan.SessionMaxSeconds,  agent.SessionMaxSeconds,  300),
        MaxTokens:   coalesce(plan.SessionMaxTokens,   agent.SessionMaxTokens,   0),
        MaxSessions: coalesce(plan.SessionMaxSessions, agent.SessionMaxSessions, 1),
    }
    if limits.MaxSessions < 1 {
        limits.MaxSessions = 1
    }
    return limits
}

// coalesce returns the first non-zero value.
func coalesce(vals ...int) int { ... }
```

### Modified `ExecuteRunStreaming` streaming loop

```
ExecuteRunStreaming(ctx, runID, inputs, onChunk):

1–8. [existing: validate, load agent+provider, build user message, transition to running]

9.  limits := resolveSessionLimits(agent, workPlan)   // workPlan may be nil → agent defaults only

10. var (
        buf        strings.Builder
        tokenCount int
    )

    var streamCtx context.Context
    var cancel    context.CancelFunc
    if limits.MaxSeconds > 0 {
        streamCtx, cancel = context.WithTimeout(ctx, time.Duration(limits.MaxSeconds)*time.Second)
    } else {
        streamCtx, cancel = context.WithCancel(ctx)
    }
    defer cancel()

    onChunkWrapped := func(chunk string) {
        buf.WriteString(chunk)
        tokenCount += estimateTokens(chunk)   // rough estimate: len(chunk)/4
        onChunk(chunk)                         // forward to caller
        if limits.MaxTokens > 0 && tokenCount >= limits.MaxTokens {
            cancel()  // triggers context deadline on next read
        }
    }

    inputTok, outputTok, err := m.callLLM(streamCtx, provider, agent,
        systemPrompt, userMessage, onChunkWrapped)

11. isYield := false
    if err != nil {
        isYield = errors.Is(err, context.DeadlineExceeded) ||
                  errors.Is(err, context.Canceled)
    }

12a. isYield == true AND segment_number < limits.MaxSessions:
       → yieldRun(ctx, run, buf.String(), tokenCount)   // see Yield Path below

12b. isYield == true AND segment_number >= limits.MaxSessions:
       → failRun(ctx, run, "max sessions reached", buf.String())
       → publisher.Publish(ctx, "ai.task.failed", failedPayload)

12c. err != nil (non-yield error):
       → [existing failure path]

12d. err == nil:
       → [existing success path]
```

### Yield Path (`yieldRun`)

```
yieldRun(ctx, run, partialOutput, tokensUsed):

1. dm.UpdateEntity(ctx, run.ID, {
       "status":         "yielded",
       "partial_output": partialOutput,
       "output_tokens":  tokensUsed,
       "completed_at":   now,
   })

2. publisher.Publish(ctx, "ai.task.yielded", {
       "task_id":        run.TaskID,
       "run_id":         run.ID,
       "chain_id":       run.ChainID,
       "segment_number": run.SegmentNumber,
       "tokens_used":    tokensUsed,
       "partial_output": partialOutput,
   })
   // publish errors: log, do not return

3. successor := dm.CreateEntity(ctx, AgentRun{
       AgentID:       run.AgentID,
       TaskID:        run.TaskID,
       Instructions:  run.Instructions,
       ChainID:       run.ChainID,
       SegmentNumber: run.SegmentNumber + 1,
       Status:        "pending_execution",
   })
   dm.CreateRelationship(ctx, {FromID: successor.ID, ToID: run.ID, Label: "continues_from"})

4. history := loadChainHistory(ctx, run.ChainID)  // see MVP-AI-023

5. ExecuteRunStreaming(ctx, successor.ID, nil, onChunk)  // recursive — history replayed in prompt
```

### Acceptance Tests

| Test | Expected |
|------|----------|
| Session hits `max_seconds` before completing | Run status = `yielded`; `partial_output` non-empty; `ai.task.yielded` published |
| Session hits `max_tokens` before completing | Same as above |
| `segment_number == max_sessions`, limit hit | `ai.task.failed` published directly; no `ai.task.yielded` |
| `max_sessions = 1` (default) | No yield — existing `failed` path on timeout/token limit |
| `onChunk` callback still receives all chunks before yield | ✅ |
| Publish failure on `ai.task.yielded` | Logged, not returned to caller |

---

## MVP-AI-023 — History Replay & Chain Management

**Status**: 🔲 Not Started
**Branch**: `feature/AI-023_chain_history`
**Depends on**: MVP-AI-022

### Goal

Load all prior segment outputs for a chain and construct the multi-turn conversation history
passed to the LLM in session N+1.

### Chain History Load

```go
// loadChainHistory returns all YIELDED runs in the chain ordered by segment_number.
func (m *aiManager) loadChainHistory(ctx context.Context, chainID string) ([]AgentRun, error) {
    runs, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
        AgencyID: m.agencyID,
        TypeID:   "AgentRun",
        Properties: map[string]any{
            "chain_id": chainID,
        },
    })
    // sort by segment_number ascending
    sort.Slice(runs, func(i, j int) bool {
        return runs[i].SegmentNumber < runs[j].SegmentNumber
    })
    return runs, err
}
```

### Conversation History Construction

Each prior segment contributes one turn to the conversation:

```
System: {agent.SystemPrompt}

User (session 1): {original instructions + event payload}
Assistant (session 1): {run[0].PartialOutput}

User (session 2): {same instructions — LLM sees it as continuing the same task}
Assistant (session 2): {run[1].PartialOutput}

...

User (session N): {instructions}
← LLM continues from here
```

This is passed to `callLLM` as a `[]ConversationTurn` extension to the existing
`systemPrompt + userMessage` shape. See [llm-client/dispatcher.md](llm-client/dispatcher.md)
for the message construction layer.

### Chain ID Allocation

- Session 1 (segment_number = 1): `chain_id = newUUID()`. Stored on the `AgentRun` entity.
- Sessions 2..N: `chain_id` copied from the predecessor run before creating the successor entity.
- Runs that are not part of a chain: `chain_id = ""` (zero value — no change to existing runs).

### `continues_from` Edge

| From | To | Label |
|------|----|-------|
| Session N+1 `AgentRun` | Session N `AgentRun` | `continues_from` |

The edge is created in `yieldRun` immediately after the successor entity is written.
Walking the chain (for debugging) traverses `continues_from` edges; queries use `chain_id`
for O(1) access.

### Acceptance Tests

| Test | Expected |
|------|----------|
| `loadChainHistory` returns runs sorted by `segment_number` | ✅ |
| Session 2 receives session 1 output in conversation history | ✅ (verified via httptest.Server capturing the LLM request body) |
| `continues_from` edge written between session 2 and session 1 | ✅ |
| `chain_id` is identical across all runs in a chain | ✅ |
| Runs with no `chain_id` are unaffected | ✅ |

---

## MVP-AI-024 — Yielded Sessions Tests

**Status**: 🔲 Not Started
**Branch**: `feature/AI-024_yielded_tests`
**Depends on**: MVP-AI-021, MVP-AI-022, MVP-AI-023

### Test File

`yielded_test.go` — covers the full yielded session lifecycle.

### Test Scenarios

| Test | Setup | Expected |
|------|-------|----------|
| Wall-clock yield | `max_seconds=1`; httptest.Server delays response | Run status = `yielded`; `ai.task.yielded` published; successor run created |
| Token yield | `max_tokens=10`; httptest.Server streams 20 tokens | Same as above |
| Two-session chain completes on session 2 | `max_sessions=2`; session 1 yields; session 2 returns valid `actions` block | Session 2 run status = `completed`; `ai.task.yielded` + `ai.run.completed` both published |
| Max sessions exhausted | `max_sessions=2`; both sessions yield | Session 2 status = `failed`; `ai.task.failed` published; no `ai.task.yielded` on session 2 |
| Work plan overrides agent `max_seconds` | Agent: `max_seconds=300`; WorkPlan: `max_seconds=5` | Effective limit = 5s |
| Work plan partial override | Agent: `max_seconds=10, max_tokens=0`; WorkPlan: `max_tokens=50` | Effective: `max_seconds=10, max_tokens=50` |
| Session 2 LLM request contains session 1 output in history | Inspect httptest.Server request body | History turn present with session 1 partial output |
| `continues_from` edge exists after yield | Query fakeDataManager relationships | Edge from session 2 run → session 1 run |
| `chain_id` same across chain | Check `chain_id` on both run entities | Equal |
| `max_sessions=1` (default) — timeout hits | Existing agent, no `max_sessions` set | `ai.task.failed` directly; no yield; backward-compatible |
