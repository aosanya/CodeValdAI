# MVP — CodeValdAI

## Goal

Deliver a production-ready AI agent management and execution gRPC microservice with ArangoDB
persistence, data-driven LLM provider configuration, and CodeValdCross registration.

---

## MVP Scope

The MVP delivers:
1. The `AIManager` Go interface (convenience-method facade over `entitygraph.DataManager`) and its `aiManager` implementation
2. The `LLMProvider`, `Agent`, `AgentRun`, `RunField`, `RunInput` domain models with run lifecycle enforcement
3. `LLMProvider` as a first-class graph entity — API key and provider type stored in ArangoDB; no injected interface
4. An ArangoDB `entitygraph.DataManager` + `AISchemaManager` implementation with `ai_` prefixed collections
5. A pre-delivered AI schema (`DefaultAISchema`) seeded on startup — TypeDefinitions for LLMProvider, Agent, AgentRun, RunField, RunInput
6. An `AIService` gRPC service exposing provider catalogue, agent catalogue, and run lifecycle RPCs
7. HTTP convenience routes registered with CodeValdCross and proxied to callers
8. CodeValdCross heartbeat registration and event publishing (`run.completed`, `run.failed`, `agent.created`)
9. Integration tests for all gRPC operations and both run phases

---

## Task List

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-AI-001 | Module scaffolding — go.mod, Makefile, buf.yaml, directory structure | ✅ Done | — |
| MVP-AI-002 | Domain models — `models.go` (LLMProvider, Agent, AgentRun, AgentRunStatus, RunField, RunInput, request/filter types) | ✅ Done | MVP-AI-001 |
| MVP-AI-003 | Error types — `errors.go` (add ErrProviderNotFound, ErrProviderInUse, ErrInvalidProvider, ErrAgentHasActiveRuns, ErrInvalidLLMResponse) | ✅ Done | MVP-AI-001 |
| MVP-AI-004 | Pre-delivered schema — `schema.go` (`DefaultAISchema` — add LLMProvider TypeDef, update Agent/AgentRun) | ✅ Done | MVP-AI-002 |
| MVP-AI-005 | `AIManager` interface & `aiManager` stub — `ai.go` (add Provider CRUD methods; remove llmClient param) | ✅ Done | MVP-AI-002, MVP-AI-003 |
| MVP-AI-006 | Delete `internal/llm/` — remove LLMClient interface and Anthropic implementation | ✅ Done | MVP-AI-005 |
| MVP-AI-007 | ArangoDB backend — `storage/arangodb/` (`storage.go`, `docs.go`, `ops.go`) | ✅ Done | MVP-AI-004, MVP-AI-005 |
| MVP-AI-008 | gRPC proto — `proto/codevaldai/v1/ai.proto` + `buf generate` (add provider RPCs) | ✅ Done | MVP-AI-005 |
| MVP-AI-009 | gRPC server — `internal/server/server.go`, `entity_server.go`, `errors.go` | ✅ Done | MVP-AI-008 |
| MVP-AI-010 | Config & registrar — `internal/config/config.go` + `internal/registrar/registrar.go` | ✅ Done | MVP-AI-001 |
| MVP-AI-011 | `cmd/main.go` wiring — inject `DataManager`, `AISchemaManager`, seed schema, start server | ✅ Done | MVP-AI-007, MVP-AI-009, MVP-AI-010 |
| MVP-AI-012 | Intake flow — `IntakeRun` implementation (fetch LLMProvider from graph; LLM infers fields; stores AgentRun + RunFields) | 🔲 Not Started | MVP-AI-005, MVP-AI-017 |
| MVP-AI-013 | Execute flow — `ExecuteRun` implementation (validate inputs, fetch LLMProvider, call LLM, store output, publish events) | 🔲 Not Started | MVP-AI-012 |
| MVP-AI-014 | Provider CRUD — `CreateProvider`, `GetProvider`, `ListProviders`, `UpdateProvider`, `DeleteProvider` implementations in `ai.go` | ✅ Done | MVP-AI-005, MVP-AI-007 |
| MVP-AI-015 | Unit & integration tests — `fakeDataManager`, full run-phase acceptance tests | 🔲 Not Started | MVP-AI-007, MVP-AI-012, MVP-AI-013 |
| MVP-AI-016 | LLMProvider/Agent schema additions — `ProviderRoute` (HF backend pin), `TimeoutSeconds` (per-Agent override), `huggingface` provider type | 🔲 Not Started | MVP-AI-002, MVP-AI-004 |
| MVP-AI-017 | LLM dispatcher refactor — `callOpenAICompatible` (OpenAI + HuggingFace), `callAnthropic`, per-Agent timeout, startup `running`-run sweep with `run.failed` publish | 🔲 Not Started | MVP-AI-016 |
| MVP-AI-018 | Streaming RPC — `ExecuteRunStreaming` server-streaming gRPC, dispatcher chunk callback, dual unary+streaming entrypoints sharing one dispatcher | 🔲 Not Started | MVP-AI-017 |

---

## Execution Order

```
MVP-AI-001
    ├── MVP-AI-002 → MVP-AI-003 → MVP-AI-004 → MVP-AI-005
    │                                               ├── MVP-AI-006
    │                                               ├── MVP-AI-007 → MVP-AI-011
    │                                               └── MVP-AI-008 → MVP-AI-009 → MVP-AI-011
    ├── MVP-AI-010 → MVP-AI-011
    └── MVP-AI-005 + MVP-AI-007 → MVP-AI-014
                   MVP-AI-002 + MVP-AI-004 → MVP-AI-016 → MVP-AI-017 → MVP-AI-012 → MVP-AI-013
                                                                ↘ MVP-AI-018 (streaming RPC; can land after 013)
                   MVP-AI-015 (after 007, 012, 013)
```

The HuggingFace + DeepSeek V4 work is split across MVP-AI-016/017/018 with
strict ordering: schema first, dispatcher second, streaming RPC third. See
[mvp-details/llm-client/README.md](mvp-details/llm-client/README.md) for
the architecture and per-task detail files.

---

## Success Criteria

- [ ] `go build ./...` succeeds
- [ ] `go test -race ./...` all pass
- [ ] `go vet ./...` shows 0 issues
- [ ] `DefaultAISchema()` seeds into `ai_schemas` on startup (idempotent)
- [ ] `CreateProvider` stores api_key + provider_type in ArangoDB
- [ ] `CreateAgent` validates required fields (including provider_id), writes `uses_provider` edge, and publishes `cross.ai.{agencyID}.agent.created`
- [ ] `IntakeRun` fetches `LLMProvider` via graph edge, calls the LLM, parses the field schema, stores `AgentRun` in `pending_intake`, and returns `run_id` + fields
- [ ] `ExecuteRun` transitions through the full status lifecycle and stores output
- [ ] `cross.ai.{agencyID}.run.completed` is published after successful execution
- [ ] `cross.ai.{agencyID}.run.failed` is published when LLM call fails
- [ ] All `AIService` RPCs work end-to-end with ArangoDB
- [ ] HTTP routes registered with CodeValdCross and proxied correctly
- [ ] CodeValdCross registration fires on startup and repeats every 20 s
- [ ] `internal/llm/` package is deleted — no `LLMClient` interface anywhere
- [ ] `LLMProvider` accepts `provider_type: "huggingface"` with optional `provider_route` backend pin
- [ ] `Agent.TimeoutSeconds` overrides system default (5 min); `cross.ai.{agencyID}.run.failed` published on `context.DeadlineExceeded`
- [ ] Startup `ReconcileRunningRuns` transitions any `running` run to `failed` with `error_message = "interrupted by service restart"` and publishes `cross.ai.{agencyID}.run.failed`
- [ ] `ExecuteRunStreaming` server-streaming RPC delivers chunks live; persists the same `AgentRun` as unary `ExecuteRun`
- [ ] DeepSeek V4 via HuggingFace Router (`deepseek-ai/DeepSeek-V4` model id, optional `:fireworks-ai` route) end-to-end run succeeds

---

## Branch Naming

Proposed branch per task. Completed tasks landed on the actual branches
recorded in [mvp_done.md](mvp_done.md) — several were bundled onto shared
branches (e.g. 002–006 + 014 all landed on `feature/AI-007_anthropic-implementation`).

```
feature/AI-001_module_scaffolding
feature/AI-002_domain_models
feature/AI-003_error_types
feature/AI-004_pre_delivered_schema
feature/AI-005_aimanager_interface
feature/AI-006_delete_llm_package
feature/AI-007_arangodb_backend
feature/AI-008_grpc_proto
feature/AI-009_grpc_server
feature/AI-010_config_registrar
feature/AI-011_cmd_wiring
feature/AI-012_intake_flow
feature/AI-013_execute_flow
feature/AI-014_provider_crud
feature/AI-015_tests
feature/AI-016_schema_huggingface_timeout
feature/AI-017_dispatcher_timeout
feature/AI-018_streaming_rpc
```
