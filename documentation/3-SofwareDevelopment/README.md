# CodeValdAI — Software Development

## Overview

This directory contains development tracking, implementation notes, and per-task
specifications for **CodeValdAI** — the AI agent management and execution microservice.

**Module**: `github.com/aosanya/CodeValdAI`
**Language**: Go 1.21+
**Storage**: ArangoDB
**LLM Providers**: Anthropic, OpenAI, HuggingFace (incl. DeepSeek V4 via Router) — data-driven via the `LLMProvider` graph entity. HuggingFace lands with [MVP-AI-016](mvp-details/llm-client/schema.md)+. See [mvp-details/llm-client/](mvp-details/llm-client/README.md).
**Registration**: CodeValdCross `OrchestratorService.Register`

---

## Index

| Document | Description |
|---|---|
| [mvp.md](mvp.md) | Full MVP scope, task list, success criteria — canonical task source |
| [mvp_done.md](mvp_done.md) | Completed tasks with completion dates and branches |
| [mvp-details/](mvp-details/README.md) | Per-topic task specifications |

---

## MVP Status

Source of truth: [mvp.md](mvp.md) (active) + [mvp_done.md](mvp_done.md) (completed).

| Task ID | Title | Status |
|---|---|---|
| MVP-AI-001 | Module scaffolding | ✅ Done |
| MVP-AI-002 | Domain models | ✅ Done |
| MVP-AI-003 | Error types | ✅ Done |
| MVP-AI-004 | Pre-delivered schema | ✅ Done |
| MVP-AI-005 | AIManager interface | ✅ Done |
| MVP-AI-006 | Delete `internal/llm/` (LLM dispatch is data-driven) | ✅ Done |
| MVP-AI-007 | ArangoDB backend | ✅ Done |
| MVP-AI-008 | gRPC proto | ✅ Done |
| MVP-AI-009 | gRPC server | ✅ Done |
| MVP-AI-010 | Config & registrar | ✅ Done |
| MVP-AI-011 | `cmd/main.go` wiring | ✅ Done |
| MVP-AI-012 | Intake flow | 🔲 Not Started |
| MVP-AI-013 | Execute flow | 🔲 Not Started |
| MVP-AI-014 | Provider CRUD | ✅ Done |
| MVP-AI-015 | Unit & integration tests | 🔲 Not Started |
| MVP-AI-016 | LLMProvider/Agent schema additions (`ProviderRoute`, `TimeoutSeconds`, `huggingface` provider type) | 🔲 Not Started |
| MVP-AI-017 | LLM dispatcher refactor (`callOpenAICompatible` + `callAnthropic`, timeout, boot sweep) | 🔲 Not Started |
| MVP-AI-018 | Streaming RPC (`ExecuteRunStreaming`) | 🔲 Not Started |

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

---

## Task Detail Files

| File | Tasks |
|---|---|
| [mvp-details/scaffolding.md](mvp-details/scaffolding.md) | MVP-AI-001 through MVP-AI-005 |
| [mvp-details/agent-management.md](mvp-details/agent-management.md) | MVP-AI-007 ArangoDB backend, MVP-AI-008 gRPC proto, MVP-AI-009 gRPC server, MVP-AI-010 Config & registrar, MVP-AI-011 cmd/main.go wiring, MVP-AI-014 Provider CRUD |
| [mvp-details/run-intake.md](mvp-details/run-intake.md) | MVP-AI-012 Intake flow |
| [mvp-details/run-execution.md](mvp-details/run-execution.md) | MVP-AI-013 Execute flow, MVP-AI-015 Tests |
| [mvp-details/llm-client/](mvp-details/llm-client/README.md) | MVP-AI-016 schema/timeout, MVP-AI-017 dispatcher refactor + boot sweep, MVP-AI-018 streaming RPC. Per-provider details in [providers/](mvp-details/llm-client/providers/). |

> **Note**: [mvp-details/agent-management.md](mvp-details/agent-management.md)'s own task-ID labels predate the renumbering and may not match this table. The task descriptions above reflect the canonical `mvp.md` IDs.

---

## Key Interfaces

| Interface | Methods |
|---|---|
| `AIManager` | Provider CRUD: `CreateProvider`, `GetProvider`, `ListProviders`, `UpdateProvider`, `DeleteProvider`. Agent CRUD: `CreateAgent`, `GetAgent`, `ListAgents`, `UpdateAgent`, `DeleteAgent`. Run lifecycle: `IntakeRun`, `ExecuteRun`, `ExecuteRunStreaming` (post-MVP-AI-018), `GetRun`, `ListRuns` |
| `AISchemaManager` | alias for `entitygraph.SchemaManager` |
| `CrossPublisher` | Publishes `cross.ai.{agencyID}.*` events to CodeValdCross |

LLM dispatch has **no interface** — `aiManager.callLLM` switches on
`LLMProvider.ProviderType` from the graph entity. See
[mvp-details/llm-client/dispatcher.md](mvp-details/llm-client/dispatcher.md).

---

## Cross-Service Events (Produced)

| Topic | Trigger |
|---|---|
| `cross.ai.{agencyID}.agent.created` | Successful `CreateAgent` |
| `cross.ai.{agencyID}.run.completed` | Successful `ExecuteRun` / `ExecuteRunStreaming` |
| `cross.ai.{agencyID}.run.failed` | Failed `ExecuteRun`, timeout, or boot-sweep reconciliation |

Inbound subscription topics (`cross.agency.created`, `work.task.dispatched`)
are not in MVP scope — see Future Work in
[mvp-details/run-execution.md](mvp-details/run-execution.md).
