```markdown
# CodeValdAI — Software Development

## Overview

This directory contains development tracking, implementation notes, and per-task
specifications for **CodeValdAI** — the AI agent management and execution microservice.

**Module**: `github.com/aosanya/CodeValdAI`  
**Language**: Go 1.21+  
**Storage**: ArangoDB  
**LLM Provider (MVP)**: Anthropic  
**Registration**: CodeValdCross `OrchestratorService.Register`

---

## Index

| Document | Description |
|---|---|
| [mvp.md](mvp.md) | Full MVP scope, task list, and success criteria |
| [mvp-details/](mvp-details/README.md) | Per-topic task specifications |

---

## MVP Status

| Task ID | Title | Status |
|---|---|---|
| MVP-AI-001 | Module scaffolding | 🔲 Not Started |
| MVP-AI-002 | Domain models | 🔲 Not Started |
| MVP-AI-003 | Error types | 🔲 Not Started |
| MVP-AI-004 | Pre-delivered schema | 🔲 Not Started |
| MVP-AI-005 | AIManager interface | 🔲 Not Started |
| MVP-AI-006 | LLMClient interface | 🔲 Not Started |
| MVP-AI-007 | Anthropic implementation | 🔲 Not Started |
| MVP-AI-008 | ArangoDB backend | 🔲 Not Started |
| MVP-AI-009 | gRPC proto | 🔲 Not Started |
| MVP-AI-010 | gRPC server | 🔲 Not Started |
| MVP-AI-011 | Config & registrar | 🔲 Not Started |
| MVP-AI-012 | cmd/main.go wiring | 🔲 Not Started |
| MVP-AI-013 | Intake flow | 🔲 Not Started |
| MVP-AI-014 | Execute flow | 🔲 Not Started |
| MVP-AI-015 | Auto-dispatch consumer | 🔲 Not Started |
| MVP-AI-016 | Unit & integration tests | 🔲 Not Started |

---

## Execution Order

```
MVP-AI-001 → MVP-AI-002 → MVP-AI-003 → MVP-AI-005
          → MVP-AI-006 → MVP-AI-007
          → MVP-AI-004 → MVP-AI-008 → MVP-AI-012
          → MVP-AI-009 → MVP-AI-010 → MVP-AI-012
          → MVP-AI-011 → MVP-AI-012
                         MVP-AI-013 → MVP-AI-014 → MVP-AI-015
                         MVP-AI-016
```

---

## Task Detail Files

| File | Tasks |
|---|---|
| [mvp-details/scaffolding.md](mvp-details/scaffolding.md) | MVP-AI-001 through MVP-AI-005 |
| [mvp-details/llm-client.md](mvp-details/llm-client.md) | MVP-AI-006, MVP-AI-007 |
| [mvp-details/agent-management.md](mvp-details/agent-management.md) | MVP-AI-008 through MVP-AI-012 |
| [mvp-details/run-intake.md](mvp-details/run-intake.md) | MVP-AI-013 |
| [mvp-details/run-execution.md](mvp-details/run-execution.md) | MVP-AI-014, MVP-AI-015, MVP-AI-016 |

---

## Key Interfaces

| Interface | Methods |
|---|---|
| `AIManager` | `CreateAgent`, `GetAgent`, `ListAgents`, `DeleteAgent`, `IntakeRun`, `ExecuteRun`, `GetRun`, `ListRuns` |
| `LLMClient` | `Complete` |
| `AISchemaManager` | alias for `entitygraph.SchemaManager` |

## Cross-Service Events

| Topic | Direction | Trigger |
|---|---|---|
| `cross.ai.{agencyID}.agent.created` | Produces | Successful `CreateAgent` |
| `cross.ai.{agencyID}.run.completed` | Produces | Successful `ExecuteRun` |
| `cross.ai.{agencyID}.run.failed` | Produces | Failed `ExecuteRun` |
| `cross.agency.created` | Consumes | Seed `DefaultAISchema` for new agency |
| `work.task.dispatched` | Consumes | Auto-trigger `IntakeRun` + `ExecuteRun` |
```
