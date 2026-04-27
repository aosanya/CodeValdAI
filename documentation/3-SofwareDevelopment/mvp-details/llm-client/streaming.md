# MVP-AI-018 — Streaming RPC

**Status**: 🔲 Not Started
**Branch**: `feature/AI-018_streaming_rpc`

**Depends on**: [MVP-AI-017](dispatcher.md) (dispatcher must accept the `onChunk` callback)

### Goal

Add a server-streaming gRPC RPC `ExecuteRunStreaming` that forwards LLM
output chunks to the client in real time. Coexists with the existing unary
`ExecuteRun` — both share the same internal dispatcher, differ only in how
they consume the chunk stream.

### When to Use Each RPC

| RPC | Use Case |
|---|---|
| `ExecuteRun` (unary) | Agent-to-agent calls, batch processing, automated dispatch |
| `ExecuteRunStreaming` | Human-facing UI, debugging, log inspection |

Both write the same `AgentRun.Output`, transition through the same FSM
(`pending_intake → pending_execution → running → completed/failed`), and
emit the same `cross.ai.{agencyID}.run.completed` / `run.failed` events.
The streaming RPC does **not** publish per-chunk Cross events — the gRPC
stream is the live channel.

---

### File: `proto/codevaldai/v1/ai.proto`

```protobuf
service AIService {
    // ...existing RPCs...

    // ExecuteRun runs the LLM call and returns the final AgentRun once
    // it reaches a terminal state (completed or failed).
    rpc ExecuteRun(ExecuteRunRequest) returns (AgentRun);

    // ExecuteRunStreaming runs the LLM call and streams output chunks to
    // the client as they arrive from the provider. Sends a final
    // ExecuteRunStreamingResponse with the terminal AgentRun once the
    // stream completes. The AgentRun is also persisted exactly as in
    // ExecuteRun — the stream is supplemental, not the source of truth.
    rpc ExecuteRunStreaming(ExecuteRunRequest) returns (stream ExecuteRunStreamingResponse);
}

message ExecuteRunStreamingResponse {
    oneof payload {
        // chunk is a partial output fragment from the LLM. Multiple chunks
        // arrive in order; concatenated they equal the final AgentRun.output.
        string chunk = 1;

        // run is the terminal AgentRun, sent once after the last chunk.
        AgentRun run = 2;
    }
}
```

Run `buf generate` to regenerate `gen/go/`. The server file
`internal/server/server.go` gains an `ExecuteRunStreaming` method.

---

### File: `internal/server/server.go`

```go
func (s *Server) ExecuteRunStreaming(
    req *pb.ExecuteRunRequest,
    stream pb.AIService_ExecuteRunStreamingServer,
) error {
    onChunk := func(c string) {
        // Send errors are non-fatal — dispatcher must still drive the run
        // to a terminal state. See "Stream Failure Modes" below.
        _ = stream.Send(&pb.ExecuteRunStreamingResponse{
            Payload: &pb.ExecuteRunStreamingResponse_Chunk{Chunk: c},
        })
    }
    run, err := s.manager.ExecuteRunStreaming(
        stream.Context(), req.RunId, toRunInputs(req.Inputs), onChunk)
    if err != nil {
        return mapErr(err)
    }
    return stream.Send(&pb.ExecuteRunStreamingResponse{
        Payload: &pb.ExecuteRunStreamingResponse_Run{Run: toProtoRun(run)},
    })
}
```

---

### File: `ai.go` — `AIManager` Surface

`AIManager` gains a sibling method that takes the chunk callback. The
existing `ExecuteRun` becomes a thin wrapper around it:

```go
type AIManager interface {
    // ...existing...

    // ExecuteRun runs the LLM call and returns the terminal AgentRun.
    // Equivalent to ExecuteRunStreaming with a buffering callback.
    ExecuteRun(ctx context.Context, runID string, inputs []RunInput) (AgentRun, error)

    // ExecuteRunStreaming runs the LLM call and invokes onChunk once per
    // streamed token group from the provider. Returns the terminal
    // AgentRun. The accumulated chunks equal AgentRun.Output.
    ExecuteRunStreaming(
        ctx context.Context,
        runID string,
        inputs []RunInput,
        onChunk func(string),
    ) (AgentRun, error)
}
```

Implementation sketch:

```go
func (m *aiManager) ExecuteRun(
    ctx context.Context, runID string, inputs []RunInput,
) (AgentRun, error) {
    return m.ExecuteRunStreaming(ctx, runID, inputs, func(string) {})
}

func (m *aiManager) ExecuteRunStreaming(
    ctx context.Context,
    runID string,
    inputs []RunInput,
    onChunk func(string),
) (AgentRun, error) {
    // 1. Validate run is in pending_intake (per MVP-AI-013)
    // 2. Store inputs, transition pending_intake → pending_execution → running
    // 3. Build user message
    // 4. Wrap onChunk to also accumulate into a strings.Builder for AgentRun.Output:
    var output strings.Builder
    wrapped := func(s string) {
        output.WriteString(s)
        onChunk(s)
    }
    inTok, outTok, err := m.callLLM(ctx, provider, agent, system, userMsg, wrapped)
    // 5. On success: write output=output.String(), status=completed, publish run.completed
    // 6. On error: write error_message, status=failed, publish run.failed
}
```

---

### Stream Failure Modes

| Failure | Handling |
|---|---|
| Provider connection drops mid-stream | Run → `failed`; `error_message = "stream interrupted: <err>"`; `run.failed` published; gRPC stream returns error to client |
| Client disconnects mid-stream (`stream.Context().Done()`) | `callLLM` ctx is cancelled; HTTP request to provider aborted; run → `failed`; `error_message = "client cancelled"`; `run.failed` published |
| Timeout fires mid-stream | Same as MVP-AI-017 timeout path; partial accumulated output written to `AgentRun.Output` |
| `onChunk` send fails (gRPC stream broken) | Logged once; dispatcher continues consuming the provider response so the run still reaches a terminal state and persists |

The principle: **the persisted `AgentRun` is the source of truth.** The
gRPC stream is convenience for live observers. Run state must reach a
terminal value regardless of stream outcome.

---

### Acceptance Tests

| Test | Expected |
|---|---|
| `ExecuteRunStreaming` with successful LLM | Multiple `chunk` messages followed by one `run` message; `AgentRun.Status = completed` |
| `ExecuteRun` (unary) with successful LLM | Single `AgentRun` returned; `Output` equals concatenated chunks |
| `ExecuteRunStreaming` with provider error | Stream returns gRPC error; `AgentRun.Status = failed`; `run.failed` published |
| `ExecuteRunStreaming` client cancels mid-stream | `AgentRun.Status = failed`; partial output preserved; `run.failed` published |
| `ExecuteRunStreaming` timeout fires | Partial chunks delivered; final state `failed`; `error_message` contains "timeout" |
| Concurrent `ExecuteRun` + `ExecuteRunStreaming` on same `runID` | Second call returns `ErrRunNotIntaked` |
| Send failure on chunk N | Logged; remaining chunks consumed from provider; run still reaches `completed`/`failed` |
