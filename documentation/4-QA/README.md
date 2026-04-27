```markdown
# CodeValdAI — QA

## Testing Strategy

CodeValdAI follows the same testing strategy as CodeValdAgency:

| Layer | Tool | Scope |
|---|---|---|
| Unit | `go test -race ./...` | All `AIManager` methods via `fakeDataManager` + `httptest.Server` mocking the provider HTTP wire shape (no `LLMClient` interface — see [../3-SofwareDevelopment/mvp-details/llm-client/](../3-SofwareDevelopment/mvp-details/llm-client/README.md)) |
| Integration | `go test -race -tags integration ./...` | Full gRPC round-trips against live ArangoDB |
| Proto | `buf lint` | Proto schema validity |
| Static analysis | `go vet ./...` | Zero issues required |
| Lint | `golangci-lint run ./...` | Zero issues required |

---

## Acceptance Criteria (MVP Gate)

- [ ] `go build ./...` succeeds
- [ ] `go test -race ./...` all pass (unit tests)
- [ ] `go vet ./...` shows 0 issues
- [ ] `DefaultAISchema()` seeds idempotently (call twice → no error)
- [ ] `CreateAgent` with missing fields returns `ErrInvalidAgent` → gRPC `INVALID_ARGUMENT`
- [ ] `IntakeRun` returns `run_id` + non-empty `fields` array
- [ ] `IntakeRun` with unknown agent returns `ErrAgentNotFound` → gRPC `NOT_FOUND`
- [ ] `ExecuteRun` on completed run returns `ErrRunNotIntaked` → gRPC `FAILED_PRECONDITION`
- [ ] `ExecuteRun` stores output and token counts correctly
- [ ] `cross.ai.{agencyID}.run.completed` published after successful execution
- [ ] `cross.ai.{agencyID}.run.failed` published after failed execution
- [ ] CodeValdCross registration fires within 30 s of startup

---

## Key Test Scenarios

### Intake Phase

| Scenario | Input | Expected |
|---|---|---|
| Happy path | Valid `agent_id` + `instructions` | `AgentRun` in `pending_intake`; `fields` non-empty |
| Missing agent | Unknown `agent_id` | `ErrAgentNotFound` |
| Empty instructions | `instructions: ""` | `ErrInvalidAgent` |
| LLM returns prose (no JSON) | `httptest.Server` returns plain text | `ErrInvalidLLMResponse` |
| LLM returns empty array | `httptest.Server` returns `"[]"` | `ErrInvalidLLMResponse` |

### Execute Phase

| Scenario | Input | Expected |
|---|---|---|
| Happy path | Valid `run_id` in `pending_intake` + inputs | `AgentRun.Status = "completed"` |
| Unknown run | Unknown `run_id` | `ErrRunNotFound` |
| Wrong status | Run in `completed` state | `ErrRunNotIntaked` |
| LLM failure | `httptest.Server` returns 500 | `AgentRun.Status = "failed"`; error returned |
| Token counts | `httptest.Server` returns `usage: {prompt_tokens:100, completion_tokens:50}` | Stored correctly |
| Timeout | `Agent.TimeoutSeconds = 1`; `httptest.Server` sleeps 2s | `AgentRun.Status = "failed"`; `error_message` contains "timeout" |
| Boot sweep | `running` run in DB on startup | After `ReconcileRunningRuns`, run is `failed` with `error_message = "interrupted by service restart"`; `run.failed` published |
| Streaming RPC | `ExecuteRunStreaming` happy path | Multiple `chunk` messages followed by terminal `run`; `AgentRun.Output` equals concatenated chunks |

> Auto-Dispatch (`work.task.dispatched` consumer) is deferred — see Future
> Work in [../3-SofwareDevelopment/mvp-details/run-execution.md](../3-SofwareDevelopment/mvp-details/run-execution.md).
```
