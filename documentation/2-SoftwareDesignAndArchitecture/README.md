```markdown
# CodeValdAI — Software Design & Architecture

## Overview

This directory contains the architecture documentation for **CodeValdAI** — the AI agent management and execution microservice.

---

## Documents

| Document | Description |
|---|---|
| [architecture.md](architecture.md) | Core design decisions and package structure (index) |
| [architecture-interfaces.md](architecture-interfaces.md) | `AIManager`, `LLMClient` interface contracts and data models |
| [architecture-graph.md](architecture-graph.md) | Graph topology, entity types, relationships, and pre-delivered schema |
| [architecture-storage.md](architecture-storage.md) | ArangoDB collections, document shapes, and indexes |
| [architecture-flows.md](architecture-flows.md) | Run lifecycle, Intake flow, Execute flow, error types, gRPC service |

---

## Key Design Decisions

| Decision | Choice |
|---|---|
| Business-logic entry point | `AIManager` interface (wraps `entitygraph.DataManager`) |
| LLM abstraction | `LLMClient` injected interface — Anthropic is the first implementation |
| Run persistence | Full `AgentRun` entity including inputs, output, token counts |
| Two-phase execution | Intake creates the run; Execute fills and processes it |
| Storage injection | `entitygraph.DataManager` + `AISchemaManager` injected by `cmd/main.go` |
| Cross registration | `OrchestratorService.Register` on startup + heartbeat every 20 s |
```
