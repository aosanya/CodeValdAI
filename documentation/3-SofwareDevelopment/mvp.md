````markdown
# MVP — CodeValdAI

## Goal

Deliver a production-ready AI agent management and execution gRPC microservice with ArangoDB
persistence, Anthropic LLM integration, and CodeValdCross registration.

---

## MVP Scope

The MVP delivers:
1. The `AIManager` Go interface (convenience-method facade over `entitygraph.DataManager`) and its `aiManager` implementation
2. The `Agent`, `AgentRun`, `RunField`, `RunInput` domain models with run lifecycle enforcement
3. A provider-agnostic `LLMClient` interface with an Anthropic concrete implementation
4. An ArangoDB `entitygraph.DataManager` + `AISchemaManager` implementation with `ai_` prefixed collections
5. A pre-delivered AI schema (`DefaultAISchema`) seeded on startup — TypeDefinitions for Agent, AgentRun, RunField, RunInput
6. An `AIService` gRPC service exposing agent catalogue and run lifecycle RPCs
7. HTTP convenience routes registered with CodeValdCross and proxied to callers
8. CodeValdCross heartbeat registration and event publishing (`run.completed`, `run.failed`, `agent.created`)
9. Auto-dispatch consumer: subscribes to `work.task.dispatched` to optionally auto-trigger runs
10. Integration tests for all gRPC operations and both run phases

---

## Task List

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-AI-001 | Module scaffolding — go.mod, Makefile, buf.yaml, directory structure | ✅ Done | — |
| MVP-AI-002 | Domain models — `models.go` (Agent, AgentRun, AgentRunStatus, RunField, RunInput, request/filter types) | ✅ Done | MVP-AI-001 |
| MVP-AI-003 | Error types — `errors.go` | ✅ Done | MVP-AI-001 |
| MVP-AI-004 | Pre-delivered schema — `schema.go` (`DefaultAISchema`) | ✅ Done | MVP-AI-002 |
| MVP-AI-005 | `AIManager` interface & `aiManager` stub — `ai.go` | 🔲 Not Started | MVP-AI-002, MVP-AI-003 |
| MVP-AI-006 | `LLMClient` interface — `internal/llm/client.go` (`LLMClient`, `CompletionRequest`, `CompletionResponse`) | 🔲 Not Started | MVP-AI-001 |
| MVP-AI-007 | Anthropic implementation — `internal/llm/anthropic/client.go` | 🔲 Not Started | MVP-AI-006 |
| MVP-AI-008 | ArangoDB backend — `storage/arangodb/` (`storage.go`, `docs.go`, `ops.go`) | 🔲 Not Started | MVP-AI-004, MVP-AI-005 |
| MVP-AI-009 | gRPC proto — `proto/codevaldai/v1/ai.proto` + `buf generate` | 🔲 Not Started | MVP-AI-005 |
| MVP-AI-010 | gRPC server — `internal/server/server.go`, `entity_server.go`, `errors.go` | 🔲 Not Started | MVP-AI-009 |
| MVP-AI-011 | Config & registrar — `internal/config/config.go` + `internal/registrar/registrar.go` | 🔲 Not Started | MVP-AI-001 |
| MVP-AI-012 | `cmd/main.go` wiring — inject `DataManager`, `LLMClient`, seed schema, start server | 🔲 Not Started | MVP-AI-008, MVP-AI-010, MVP-AI-011 |
| MVP-AI-013 | Intake flow — `IntakeRun` implementation (LLM infers fields; stores AgentRun + RunFields) | 🔲 Not Started | MVP-AI-005, MVP-AI-007 |
| MVP-AI-014 | Execute flow — `ExecuteRun` implementation (validate inputs, call LLM, store output, publish events) | 🔲 Not Started | MVP-AI-013 |
| MVP-AI-015 | Auto-dispatch consumer — subscribe to `work.task.dispatched`; auto-trigger `IntakeRun` + `ExecuteRun` | 🔲 Not Started | MVP-AI-014 |
| MVP-AI-016 | Unit & integration tests — `fakeDataManager`, `fakeLLMClient`, full run-phase acceptance tests | 🔲 Not Started | MVP-AI-008, MVP-AI-013, MVP-AI-014 |

---

## Execution Order

```
MVP-AI-001
    ├── MVP-AI-002 → MVP-AI-003 → MVP-AI-004 → MVP-AI-005
    │                                               ├── MVP-AI-008 → MVP-AI-012
    │                                               └── MVP-AI-009 → MVP-AI-010 → MVP-AI-012
    ├── MVP-AI-006 → MVP-AI-007
    │                   └── MVP-AI-013 → MVP-AI-014 → MVP-AI-015
    └── MVP-AI-011 → MVP-AI-012
                                          MVP-AI-016 (after 008, 013, 014)
```

---

## Success Criteria

- [ ] `go build ./...` succeeds
- [ ] `go test -race ./...` all pass
- [ ] `go vet ./...` shows 0 issues
- [ ] `DefaultAISchema()` seeds into `ai_schemas` on startup (idempotent)
- [ ] `CreateAgent` validates required fields and publishes `cross.ai.{agencyID}.agent.created`
- [ ] `IntakeRun` calls the Anthropic LLM, parses the field schema, stores `AgentRun` in `pending_intake`, and returns `run_id` + fields
- [ ] `ExecuteRun` transitions through the full status lifecycle and stores output
- [ ] `cross.ai.{agencyID}.run.completed` is published after successful execution
- [ ] `cross.ai.{agencyID}.run.failed` is published when LLM call fails
- [ ] All `AIService` RPCs work end-to-end with ArangoDB
- [ ] HTTP routes registered with CodeValdCross and proxied correctly
- [ ] CodeValdCross registration fires on startup and repeats every 20 s
- [ ] `work.task.dispatched` events auto-trigger runs when the consumer is active
- [ ] `LLMClient` is injected — swapping from Anthropic to another provider requires only `cmd/main.go` changes

---

## Branch Naming

```
feature/AI-001_module_scaffolding
feature/AI-002_domain_models
feature/AI-003_error_types
feature/AI-004_pre_delivered_schema
feature/AI-005_aimanager_interface
feature/AI-006_llmclient_interface
feature/AI-007_anthropic_implementation
feature/AI-008_arangodb_backend
feature/AI-009_grpc_proto
feature/AI-010_grpc_server
feature/AI-011_config_registrar
feature/AI-012_cmd_wiring
feature/AI-013_intake_flow
feature/AI-014_execute_flow
feature/AI-015_auto_dispatch
feature/AI-016_tests
```
````
