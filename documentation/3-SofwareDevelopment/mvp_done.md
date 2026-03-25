# MVP Done — CodeValdAI Completed Tasks

Completed tasks are removed from `mvp.md` and recorded here with their completion date.

| Task ID | Title | Completion Date | Branch |
|---------|-------|-----------------|--------|
| MVP-AI-001 | Module scaffolding — go.mod, Makefile, buf.yaml, directory structure | 2026-03-24 | feature/AI-001_module-scaffolding |
| MVP-AI-002 | Domain models — `models.go` (LLMProvider, Agent w/ ProviderID, AgentRun, AgentRunStatus, RunField, RunInput, request/filter types) | 2026-03-24 | feature/AI-007_anthropic-implementation |
| MVP-AI-003 | Error types — `errors.go` (ErrProviderNotFound, ErrProviderInUse, ErrInvalidProvider, ErrAgentNotFound, ErrAgentHasActiveRuns, ErrInvalidLLMResponse) | 2026-03-24 | feature/AI-007_anthropic-implementation |
| MVP-AI-004 | Pre-delivered schema — `schema.go` (`DefaultAISchema` — LLMProvider, Agent, AgentRun, RunField, RunInput TypeDefs; uses_provider/has_run/has_field/has_input edges) | 2026-03-24 | feature/AI-007_anthropic-implementation |
| MVP-AI-005 | `AIManager` interface & `aiManager` stub — `ai.go` (Provider CRUD: CreateProvider, GetProvider, ListProviders, UpdateProvider, DeleteProvider; Agent CRUD; run lifecycle stubs; no llmClient param) | 2026-03-24 | feature/AI-007_anthropic-implementation |
| MVP-AI-006 | Delete `internal/llm/` — removed LLMClient interface, CompletionRequest/Response types, and Anthropic stub; LLM dispatch is now data-driven via LLMProvider entity | 2026-03-24 | feature/AI-007_anthropic-implementation |
| MVP-AI-007 | ArangoDB backend — `storage/arangodb/` (`storage.go`, `docs.go`, `ops.go`) | 2026-03-24 | feature/AI-007_anthropic-implementation |
| MVP-AI-008 | gRPC proto — `proto/codevaldai/v1/ai.proto` + `buf generate` (add provider RPCs) | 2026-03-25 | feature/AI-008_grpc_proto |
| MVP-AI-009 | gRPC server — `internal/server/server.go`, `entity_server.go`, `errors.go` | 2026-03-25 | feature/AI-008_grpc_proto |
| MVP-AI-010 | Config & registrar — `internal/config/config.go` + `internal/registrar/registrar.go` | 2026-03-25 | feature/AI-008_grpc_proto |
| MVP-AI-011 | `cmd/main.go` wiring — inject `DataManager`, `AISchemaManager`, seed schema, start server | 2026-03-25 | feature/AI-008_grpc_proto |
| MVP-AI-014 | Provider CRUD — `CreateProvider`, `GetProvider`, `ListProviders`, `UpdateProvider`, `DeleteProvider` implementations in `ai.go` | 2026-03-24 | feature/AI-007_anthropic-implementation |
