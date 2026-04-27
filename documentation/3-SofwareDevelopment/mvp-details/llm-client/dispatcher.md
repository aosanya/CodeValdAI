# MVP-AI-017 — Dispatcher Refactor + Timeout + Boot Sweep

**Status**: 🔲 Not Started
**Branch**: `feature/AI-017_dispatcher_timeout`

**Depends on**: [MVP-AI-016](schema.md) (schema/model fields must exist first)

### Goal

Replace the single-case `callLLM` switch with the full three-provider
dispatcher, wrap every call in a per-Agent timeout, and add a startup sweep
that publishes `run.failed` for any `running` runs left behind by a previous
service restart.

After this task, MVP-AI-012 (Intake) and MVP-AI-013 (Execute) can wire up
the dispatcher cleanly.

---

### File: `ai.go` — Dispatch Switch

```go
const defaultLLMCallTimeout = 5 * time.Minute

// callLLM dispatches a completion request to the configured provider.
// Always streams from the provider internally; onChunk is called once per
// streamed token group. Pass a buffering callback for unary RPCs, or the
// gRPC stream send for streaming RPCs.
//
// Returns the input/output token counts (or 0/0 if the provider omitted
// usage info) and any error. The accumulated content is delivered via
// onChunk — it is NOT returned from this function.
func (m *aiManager) callLLM(
    ctx context.Context,
    provider LLMProvider,
    agent Agent,
    system, user string,
    onChunk func(string),
) (inputTok, outputTok int, err error) {

    timeout := defaultLLMCallTimeout
    if agent.TimeoutSeconds > 0 {
        timeout = time.Duration(agent.TimeoutSeconds) * time.Second
    }
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    switch provider.ProviderType {
    case "anthropic":
        return callAnthropic(ctx, provider, agent, system, user, onChunk)
    case "openai", "huggingface":
        return callOpenAICompatible(ctx, provider, agent, system, user, onChunk)
    default:
        return 0, 0, fmt.Errorf("unsupported provider_type %q", provider.ProviderType)
    }
}
```

Per-provider request/response shapes are documented in
[providers/anthropic.md](providers/anthropic.md) and
[providers/openai-compatible.md](providers/openai-compatible.md).

---

### Timeout Handling

When `context.DeadlineExceeded` is returned from the dispatcher, the
caller (`ExecuteRun` in MVP-AI-013) treats it identically to any other LLM
error — the only special handling is the human-readable `error_message`:

```go
inputTok, outputTok, err := m.callLLM(ctx, provider, agent, system, user, buf)
if err != nil {
    msg := err.Error()
    if errors.Is(err, context.DeadlineExceeded) {
        msg = fmt.Sprintf("timeout exceeded after %s", timeout)
    }
    m.dm.UpdateEntity(ctx, runID, map[string]any{
        "status":        "failed",
        "error_message": msg,
        "completed_at":  now,
        "updated_at":    now,
    })
    m.publisher.Publish(ctx,
        fmt.Sprintf("cross.ai.%s.run.failed", m.agencyID), runID)
    return AgentRun{Status: "failed"}, err
}
```

There is no separate `ErrLLMTimeout` sentinel — `context.DeadlineExceeded`
is the canonical signal and the `error_message` carries the human-readable
duration.

---

### Boot Sweep — Startup Reconciliation

A new function `ReconcileRunningRuns` runs in `cmd/main.go` after schema
seeding and before the gRPC server starts. It transitions any `AgentRun`
left in `running` state to `failed`.

```go
// internal/recovery/recovery.go

// ReconcileRunningRuns transitions any AgentRun left in "running" state to
// "failed" with error_message="interrupted by service restart" and
// publishes cross.ai.{agencyID}.run.failed for each. Called once on
// startup before the gRPC server begins accepting requests.
func ReconcileRunningRuns(
    ctx context.Context,
    dm entitygraph.DataManager,
    publisher CrossPublisher,
    agencyID string,
    log *slog.Logger,
) error {
    runs, err := dm.QueryEntities(ctx, entitygraph.Query{
        TypeID: "AgentRun",
        Filter: map[string]any{"status": "running"},
    })
    if err != nil {
        return fmt.Errorf("query running runs: %w", err)
    }
    now := time.Now().UTC().Format(time.RFC3339)
    for _, run := range runs {
        if err := dm.UpdateEntity(ctx, run.ID, map[string]any{
            "status":        "failed",
            "error_message": "interrupted by service restart",
            "completed_at":  now,
            "updated_at":    now,
        }); err != nil {
            log.Error("reconcile run", "run_id", run.ID, "err", err)
            continue
        }
        if err := publisher.Publish(ctx,
            fmt.Sprintf("cross.ai.%s.run.failed", agencyID), run.ID); err != nil {
            log.Warn("publish run.failed during reconcile", "run_id", run.ID, "err", err)
        }
        log.Info("reconciled interrupted run", "run_id", run.ID)
    }
    return nil
}
```

#### Multi-Replica Note

The current MVP runs as a single replica. If multi-replica is ever
introduced, the sweep above will incorrectly fail another replica's
in-flight runs. Mitigation (out of scope for MVP):

- Add `owner_instance_id` to `AgentRun` (set when transitioning to
  `running`).
- Sweep only runs whose `owner_instance_id == this instance` AND whose
  `started_at + timeout < now`.

This is a documented future-work item, not a current correctness issue.

---

### File: `cmd/main.go` — Wiring

```go
// after AISchemaManager.SetSchema, before grpcServer.Serve:
if err := recovery.ReconcileRunningRuns(ctx, dm, publisher, agencyID, log); err != nil {
    log.Error("startup reconcile", "err", err)
    // continue: reconcile failure must not block startup
}
```

---

### Token-Counting Tolerance

Some HuggingFace Router backends omit the `usage` field in streaming
responses. The dispatcher must tolerate this:

```go
// inside callOpenAICompatible streaming loop, after the [DONE] sentinel
if usageMissing {
    log.Warn("provider omitted usage; storing zero token counts",
        "provider_type", provider.ProviderType,
        "model", agent.Model)
    return 0, 0, nil
}
```

Token counts of `0` on a `completed` run mean "provider did not report
usage," not "the call did nothing." Downstream billing/metrics consumers
must handle zeros explicitly.

---

### Acceptance Tests

| Test | Expected |
|---|---|
| `callLLM` with `provider_type: "anthropic"` | Routes to `callAnthropic` |
| `callLLM` with `provider_type: "openai"` | Routes to `callOpenAICompatible` |
| `callLLM` with `provider_type: "huggingface"` | Routes to `callOpenAICompatible` |
| `callLLM` with unknown `provider_type` | `fmt.Errorf("unsupported provider_type %q", ...)` |
| `callLLM` with `Agent.TimeoutSeconds = 1` and slow provider | `context.DeadlineExceeded` after ~1s |
| `callLLM` with `Agent.TimeoutSeconds = 0` | Uses `defaultLLMCallTimeout` |
| `ReconcileRunningRuns` with one `running` run | Run becomes `failed`; `run.failed` published |
| `ReconcileRunningRuns` with no `running` runs | No-op; no publish calls |
| `ReconcileRunningRuns` with publisher failure | Logged warning; reconcile continues to next run |
| Provider response missing `usage` | Run completes; `input_tokens=0`, `output_tokens=0`; warning logged |
