```markdown
# CodeValdAI — mvp-details Index

| File | Tasks Covered |
|---|---|
| [scaffolding.md](scaffolding.md) | MVP-AI-001 Module scaffolding, MVP-AI-002 Domain models, MVP-AI-003 Error types, MVP-AI-004 Pre-delivered schema, MVP-AI-005 AIManager interface |
| [llm-client/](llm-client/README.md) | MVP-AI-016 schema/timeout, MVP-AI-017 dispatcher refactor + boot sweep, MVP-AI-018 streaming RPC. Per-provider details in [providers/](llm-client/providers/) (Anthropic, OpenAI/HuggingFace incl. DeepSeek V4). MVP-AI-006 (delete `internal/llm/`) is now ✅ Done — see [../mvp_done.md](../mvp_done.md). |
| [agent-management.md](agent-management.md) | MVP-AI-008 ArangoDB backend, MVP-AI-009 gRPC proto, MVP-AI-010 gRPC server, MVP-AI-011 Config & registrar, MVP-AI-012 cmd/main.go wiring |
| [run-intake.md](run-intake.md) | MVP-AI-013 Intake flow |
| [run-execution.md](run-execution.md) | MVP-AI-014 Execute flow, MVP-AI-015 Auto-dispatch consumer, MVP-AI-016 Tests |
```
