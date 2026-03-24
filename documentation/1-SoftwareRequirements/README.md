```markdown
# CodeValdAI — Software Requirements

## Overview

This directory captures the requirements for the **CodeValdAI** microservice — the AI agent management and execution service in the CodeVald platform.

---

## Scope

CodeValdAI is responsible for:
- Persisting **Agent** configurations (model, provider, system prompt, parameters) to ArangoDB
- Running the **Intake** phase: submitting a Workflow + Instructions to the LLM and returning a structured input-field schema `[{fieldname, type, label, required, …}]`
- Running the **Execute** phase: accepting filled inputs, calling the LLM with full context, persisting the output as an `AgentRun` entity
- Publishing `cross.ai.{agencyID}.run.completed` / `.run.failed` events via CodeValdCross
- Consuming `work.task.dispatched` to optionally auto-trigger runs
- Registering with **CodeValdCross** `OrchestratorService.Register` on startup

---

## Key Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| REQ-001 | `CreateAgent` validates required fields (name, provider, model, system_prompt) before insert | P0 |
| REQ-002 | `IntakeRun` calls the LLM and returns a structured field schema; creates an `AgentRun` in `pending_intake` state | P0 |
| REQ-003 | `ExecuteRun` transitions the run `pending_intake → pending_execution → running → completed / failed` | P0 |
| REQ-004 | All operations are context-cancellable (deadline propagated to ArangoDB and LLM client) | P0 |
| REQ-005 | Service registers with CodeValdCross within 30 s of startup; heartbeat every 20 s | P0 |
| REQ-006 | gRPC server listens on `CODEVALDAI_GRPC_PORT` (default `:50056`) | P1 |
| REQ-007 | `LLMClient` is an injected interface — Anthropic is the first implementation; provider is swappable | P0 |
| REQ-008 | `cross.ai.{agencyID}.run.completed` is published after every successful `ExecuteRun` | P0 |
| REQ-009 | `cross.ai.{agencyID}.run.failed` is published when a run transitions to `failed` | P0 |
| REQ-010 | Service subscribes to `work.task.dispatched` and can auto-trigger a run when a task is dispatched | P1 |
| REQ-011 | Pre-delivered schema (`DefaultAISchema`) is seeded into `ai_schemas` on startup (idempotent) | P0 |

---

## Introduction

| Document | Description |
|---|---|
| [Introduction / Problem Definition](introduction/problem-definition.md) | What an AI Agent is; the problem CodeValdAI solves |
| [High-Level Features](introduction/high-level-features.md) | Agent catalogue, two-phase run lifecycle, LLM provider model |
| [Stakeholders & Roles](introduction/stakeholders.md) | Consumers of CodeValdAI within the platform |

---

## 🗺️ Related Documentation

| Section | Link |
|---------|------|
| Architecture | [../2-SoftwareDesignAndArchitecture/README.md](../2-SoftwareDesignAndArchitecture/README.md) |
| Development | [../3-SofwareDevelopment/README.md](../3-SofwareDevelopment/README.md) |
| QA | [../4-QA/README.md](../4-QA/README.md) |
```
