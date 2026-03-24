```markdown
# CodeValdAI — Documentation

CodeValdAI is the **AI agent management and execution** microservice in the CodeVald platform.

It owns two concerns:
1. **Agent Catalogue** — the persistent registry of AI agent configurations (model, provider, system prompt, parameters)
2. **Run Execution** — the two-phase lifecycle for running an agent against a Workflow: *Intake* (infer required inputs) → *Execute* (run with those inputs)

---

## Documentation Sections

| Section | Description |
|---|---|
| [1 — Software Requirements](1-SoftwareRequirements/README.md) | Requirements, introduction, problem definition, and stakeholders |
| [2 — Software Design & Architecture](2-SoftwareDesignAndArchitecture/README.md) | Architecture, data models, interfaces, graph topology, and service design |
| [3 — Software Development](3-SofwareDevelopment/README.md) | Development tracking, MVP task list, and per-task implementation specs |
| [4 — QA](4-QA/README.md) | Testing strategy, test cases, and quality criteria |

---

## Quick Reference

| Item | Value |
|---|---|
| **gRPC Port** | `:50056` |
| **Storage** | ArangoDB — `ai_entities`, `ai_relationships`, `ai_schemas` collections |
| **Registers with** | CodeValdCross `OrchestratorService.Register` |
| **Produces** | `cross.ai.{agencyID}.run.completed`, `cross.ai.{agencyID}.run.failed`, `cross.ai.{agencyID}.agent.created` |
| **Consumes** | `cross.agency.created`, `work.task.dispatched` |
| **LLM Provider (MVP)** | Anthropic — injected via `LLMClient` interface |
| **Module** | `github.com/aosanya/CodeValdAI` |
```
