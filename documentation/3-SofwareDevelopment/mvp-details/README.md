# CodeValdAI — mvp-details Index

Task IDs match canonical [../mvp.md](../mvp.md) and [../mvp_done.md](../mvp_done.md).

| File | Tasks Covered |
|---|---|
| [scaffolding.md](scaffolding.md) | MVP-AI-001 Module scaffolding, MVP-AI-002 Domain models, MVP-AI-003 Error types, MVP-AI-004 Pre-delivered schema, MVP-AI-005 AIManager interface — all ✅ Done |
| [agent-management.md](agent-management.md) | MVP-AI-007 ArangoDB backend, MVP-AI-008 gRPC proto, MVP-AI-009 gRPC server, MVP-AI-010 Config & registrar, MVP-AI-011 cmd/main.go wiring — all ✅ Done |
| [run-intake.md](run-intake.md) | MVP-AI-012 Intake flow |
| [run-execution.md](run-execution.md) | MVP-AI-013 Execute flow, MVP-AI-015 Tests, plus deferred Auto-Dispatch Consumer (Future Work) |
| [llm-client/](llm-client/README.md) | MVP-AI-016 schema/timeout, MVP-AI-017 dispatcher refactor + boot sweep, MVP-AI-018 streaming RPC. Per-provider details in [providers/](llm-client/providers/) (Anthropic, OpenAI/HuggingFace incl. DeepSeek V4). MVP-AI-006 (delete `internal/llm/`) is ✅ Done — see [../mvp_done.md](../mvp_done.md). |
| [cross-subscription.md](cross-subscription.md) | MVP-AI-019 Cross subscription — `NotifyEvent` RPC, `work.task.status.changed` consumer registration, acknowledgement via PubSub Ack |

> MVP-AI-014 (Provider CRUD — ✅ Done) was implemented as part of the broader
> AIManager work and is recorded in [../mvp_done.md](../mvp_done.md). It does
> not have a dedicated detail file.
