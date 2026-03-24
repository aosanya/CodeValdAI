```markdown
# CodeValdAI — QA

## Testing Strategy

CodeValdAI follows the same testing strategy as CodeValdAgency:

| Layer | Tool | Scope |
|---|---|---|
| Unit | `go test -race ./...` | All `AIManager` methods via `fakeDataManager` + `FakeLLMClient` |
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
| LLM returns prose (no JSON) | Fake LLM returns plain text | `ErrInvalidLLMResponse` |
| LLM returns empty array | Fake LLM returns `"[]"` | `ErrInvalidLLMResponse` |

### Execute Phase

| Scenario | Input | Expected |
|---|---|---|
| Happy path | Valid `run_id` in `pending_intake` + inputs | `AgentRun.Status = "completed"` |
| Unknown run | Unknown `run_id` | `ErrRunNotFound` |
| Wrong status | Run in `completed` state | `ErrRunNotIntaked` |
| LLM failure | Fake LLM returns error | `AgentRun.Status = "failed"`; error returned |
| Token counts | Fake LLM returns `{InputTokens:100, OutputTokens:50}` | Stored correctly |

### Auto-Dispatch

| Scenario | Input | Expected |
|---|---|---|
| Valid event | Well-formed `work.task.dispatched` payload | Run created and executed |
| Malformed payload | Invalid JSON | Error logged; no panic |
| Unknown agent | `agent_id` not in DB | `ErrAgentNotFound` logged; no panic |
```
