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
