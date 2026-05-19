# CodeValdAI — Action Protocol

> Part of the mvp-details. Index: [README.md](README.md)

---

## Overview

The Action Protocol is the mechanism by which an AI agent communicates **intent** to the rest of the CodeVald platform. Rather than directly calling service APIs, the LLM embeds a structured block in its output that CodeValdAI parses and publishes as PubSub events via CodeValdCross.

This keeps the LLM stateless and side-effect-free: it writes text; CodeValdAI takes action.

---

## 1. LLM Output Format

The LLM is instructed (via the system prompt) to append an `actions` fenced code block at the end of its response when it needs to trigger cross-service behaviour:

````
Some prose response here...

```actions
[
  {
    "topic": "git.branch.create",
    "payload": {
      "branch_name": "feature/fix-auth",
      "base_branch": "main"
    }
  },
  {
    "topic": "work.task.update",
    "payload": {
      "task_id": "abc-123",
      "branch_name": "feature/UTIL-001-widget"
    }
  }
]
```
````

Rules:
- The block is a JSON array of `{"topic": string, "payload": object}` objects.
- The block is **optional** — if the agent needs no side effects it simply omits it.
- The block must be the last fenced block in the output (parsing stops at the first closing fence after `\`\`\`actions`).
- The payload may contain any JSON-serialisable fields the consuming service expects.

---

## 2. Action Types (`actions.go`)

```go
// Action represents a single cross-service intent the LLM wants to trigger.
type Action struct {
    Topic   string         `json:"topic"`
    Payload map[string]any `json:"payload"`
}

// RawPayload serialises Payload back to a JSON string for the PubSub wire.
func (a Action) RawPayload() string

// CatalogueEntry describes one topic a service Consumes or Produces.
type CatalogueEntry struct {
    ServiceName string
    Topic       string
    Direction   string // "consumes" or "produces"
}
```

---

## 3. Action Dispatch (`execute.go` — `dispatchActions`)

After a successful LLM call, `ExecuteRun` calls `dispatchActions` before publishing the run-completed event:

```
1. parseActions(output) → []Action
2. For each Action:
       publisher.Publish(ctx, action.Topic, agencyID, "codevaldai", action.RawPayload())
       // best-effort: publish failures are logged, never returned to caller
3. publisher.Publish(ctx, "ai.run.completed", agencyID, "codevaldai", {"run_id":"<id>"})
```

A nil publisher (when CrossGRPCAddr is not configured) skips all publishing silently.

---

## 4. Action Catalogue

Before every LLM call, CodeValdAI fetches the live service registry from CodeValdCross and injects a formatted catalogue of all available actions into the system prompt.

### 4a. Fetching (`catalogue.go`)

```go
func FetchActionCatalogue(ctx context.Context, crossHTTPAddr, agencyID string) []CatalogueEntry
```

Calls `GET {crossHTTPAddr}/services/registry?agencyId={agencyID}` and maps the response:

```json
[
  {
    "ServiceName": "codevaldgit",
    "Consumes": ["git.branch.create", "git.file.update"],
    "Produces": ["git.branch.fetched"]
  }
]
```

Every `Consumes` topic = an action the LLM **can** trigger. `Produces` topics are shown for informational context (the LLM can see what events it may receive in future turns).

### 4b. Formatting (`actions.go` — `FormatActionCatalogue`)

The catalogue is formatted as a Markdown table injected into the system prompt:

```
## Available Actions
| Service | Topic | Direction |
|---|---|---|
| codevaldgit | git.branch.create | consumes |
| codevaldgit | git.file.update | consumes |
| codevaldgit | git.branch.fetched | produces |
```

### 4c. Wiring (`ai.go` — `buildSystemPrompt`)

```go
func (m *aiManager) buildSystemPrompt(ctx context.Context, agentSystemPrompt, instructions string) string
```

Called at the start of `ExecuteRun` (after the run enters `running` state). It concatenates:

1. `agentSystemPrompt` — the static prompt stored on the Agent entity
2. `FormatActionCatalogue(FetchActionCatalogue(ctx, m.crossHTTPAddr, m.agencyID))` — live action catalogue
3. `HydrateEventContext(ctx, m.crossHTTPAddr, m.agencyID, instructions)` — enriched event context (see §5)

If `crossHTTPAddr` is empty the catalogue and hydration steps are skipped; the agent still runs with its base system prompt.

---

## 5. Context Hydration (`contexthydrate.go`)

Before passing the event payload to the LLM, CodeValdAI enriches it with data fetched from the originating service. This gives the LLM human-readable context (task title, branch, full file contents) rather than raw UUIDs.

### 5a. Entry Point

```go
func HydrateEventContext(ctx context.Context, crossHTTPAddr, agencyID, eventPayload string) string
```

Parses `eventPayload` as a JSON map and, for each known entity ID key, fetches details:

| Key(s) in payload | Fetches from | Adds to prompt |
|---|---|---|
| `TaskID` / `task_id` / `ParentTaskID` | `GET /work/{agencyID}/tasks/{taskID}` | Title, Description, Status, Branch, Project |
| `TodoID` / `todo_id` | `GET /work/{agencyID}/todos/{todoID}` | Reads `precalls` field; executes each spec |
| *(always)* | `GET /git/{agencyID}/repositories` | Repository list; picks `project.repo_name` if set |
| *(when task has `branch_name`)* | `GET /git/{agencyID}/repositories/{repo}/branches/{branch}/tree` recursively | Branch file dictionary (≤ 50 files, ≤ 10 KB each) |

The raw payload is always included verbatim; enrichment is additive and best-effort.

### 5b. Precalls

A `TaskTodo` entity may carry a `precalls` field: a JSON-encoded `[]PrecallSpec`.
When `HydrateEventContext` detects a `TodoID`, it fetches the todo, parses its precalls, and executes each spec before building the file dictionary:

```json
[
  {
    "service": "git",
    "operation": "blob_search",
    "query": "authentication middleware",
    "label": "Files related to auth middleware"
  }
]
```

Routing is by `service`:
- `"git"` → `GET /git/{agencyID}/repositories/_/blobs/search?query=<q>&limit=20`
  (ArangoSearch View, BM25-ranked, agency-scoped)

Results are injected as a `## Precall Results` section before the file dictionary. The LLM agent then knows which files are relevant before it starts editing.

### 5c. Branch File Dictionary

When the task has a `branch_name`, up to 50 text files (≤ 10 KB each) are fetched and injected:

```
## Working Branch: `feature/UTIL-001-widget`
The following files are loaded from ArangoDB...

### `src/auth/middleware.ts`
\`\`\`
... file content ...
\`\`\`

---
**Writing files back (creates a git commit):**
\`\`\`json
{"topic":"git.file.write","payload":{"repository":"...","branch_name":"...","path":"...","content":"...","message":"..."}}
\`\`\`
```

### 5d. Project → Repo Resolution

The task's `project_name` is used to fetch the linked `Project` entity and read its `repo_name` field (set when the project is created). This ensures the correct repository is used when a project has multiple repos or when the first-listed repo is not the right one.

### 5e. Output Format

```
## Event Context
Raw payload: {"TodoID":"todo-456","TaskID":"abc-123"}
Task ID: abc-123
Task Title: Add OAuth2 support
Task Status: in_progress
Task Branch: feature/UTIL-001-oauth
Task Project: utility-app

## Precall Results
### Files related to auth middleware
Search query: `authentication middleware` — 3 result(s)
- **`src/auth/middleware.ts`** (score: 8.42)
  > export function authMiddleware(req, res, next) { ...

## Working Branch: `feature/UTIL-001-oauth`
...
```

---

## 6. Topic Naming Convention

All CodeValdAI PubSub topics follow the pattern:

```
{service}.{noun}.{verb}
```

`ai.run.completed` — not `cross.ai.…` — because Cross is routing infrastructure, not a domain service.

| Topic | Published when |
|---|---|
| `ai.agent.created` | `CreateAgent` succeeds |
| `ai.run.completed` | `ExecuteRun` succeeds |
| `ai.run.failed` | `ExecuteRun` errors or times out |

Actions dispatched from the LLM use **the consuming service's topic namespace** (e.g. `git.branch.create`, `work.task.update`), never the `ai.*` namespace.

---

## 7. End-to-End Sequence

```
event arrives (e.g. work.task.assigned)
    │
    ▼
RACI dispatcher: triggerPlanRun
    │
    ├─► IntakeRun  → run enters pending_intake
    │
    └─► ExecuteRun
            │
            ├── buildSystemPrompt
            │       ├── FetchActionCatalogue  (Cross HTTP /services/registry)
            │       └── HydrateEventContext   (Cross HTTP /work/.../tasks/...)
            │
            ├── callLLM (streaming)  → output contains optional ```actions block
            │
            ├── dispatchActions
            │       └── publisher.Publish(ctx, action.Topic, ...)  × N
            │
            └── publisher.Publish("ai.run.completed", ...)
```

---

## 8. Agent System Prompt Requirements

For the action protocol to function, each agent's `system_prompt` must instruct the LLM to:

1. Consult the action catalogue injected below the system prompt.
2. Append an `actions` block at the end of the response when cross-service side effects are needed.
3. Match topic names exactly to the catalogue — no invented topics.

Example addition for a Developer Agent:

```
When your task requires creating a branch, updating a file, or modifying a task,
append an ```actions block at the end of your response. Each entry must use a topic
from the Available Actions catalogue injected into this prompt. Do not invent topics.
```

---

## 9. Future Work

| Item | Notes |
|---|---|
| Action filtering per agent | Agents should only see the topics relevant to their role. Currently the full catalogue is passed. |
| Retry on publish failure | Actions are currently best-effort. A dead-letter queue or retry loop would improve reliability. |
| Payload validation | Verify action payloads against a per-topic schema before publishing. |
| Multi-turn actions | Agent receives feedback from dispatched events and acts again (requires stateful run sessions). |
