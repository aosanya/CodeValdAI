```markdown
# CodeValdAI — Stakeholders & Roles

## Platform Consumers

| Stakeholder | Role in CodeValdAI |
|---|---|
| **CodeValdCross** | Routes pub/sub events to and from CodeValdAI; proxies HTTP convenience routes to callers |
| **CodeValdWork** | Dispatches `work.task.dispatched` events that CodeValdAI can consume to auto-trigger runs |
| **CodeValdAgency** | Provides the Workflow and WorkItem context that the Intake phase reads to infer required fields |
| **CodeValdGit** | Stores LLM output artifacts when the caller chooses to persist run output as files |
| **CodeValdHi** | The UI client — calls Intake and Execute endpoints; displays agent catalogue and run history |

---

## Human Roles

| Role | Interaction |
|---|---|
| **Platform Operator** | Deploys CodeValdAI; configures `ANTHROPIC_API_KEY`, DB connection, and Cross address via environment |
| **Agency Administrator** | Creates and manages Agent configurations via the API; reviews run history |
| **Agent Executor** | Submits Intake requests, fills returned fields, and submits Execute requests |

---

## External Dependencies

| Dependency | Purpose |
|---|---|
| **Anthropic API** | LLM provider for MVP — all calls go through `internal/llm/anthropic/` |
| **ArangoDB** | Persistence for Agent, AgentRun, RunField, RunInput entities |
| **CodeValdCross gRPC** | Registration heartbeat and pub/sub event publishing |
| **CodeValdSharedLib** | `entitygraph.DataManager`, `registrar`, `serverutil`, `arangoutil`, `types` |
```
