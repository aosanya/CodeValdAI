---
agent: agent
---

# Start New Task

> ⚠️ **Before starting a new task**, run `CodeValdAI/.github/prompts/finish-task.prompt.md` to ensure any in-progress task is properly completed and merged first.

Follow the **mandatory task startup process** for CodeValdAI tasks:

## Task Startup Process (MANDATORY)

1. **Select the next task**
   - Check `documentation/3-SofwareDevelopment/mvp.md` for the task list and current status
   - Check `documentation/3-SofwareDevelopment/mvp-details/` for detailed specs per topic
   - Check `documentation/1-SoftwareRequirements/requirements.md` for unimplemented functional requirements
   - Prefer foundational tasks (e.g., `LLMClient` interface, agent catalogue CRUD) before dependent flows (e.g., run intake/execute)

2. **Update the prioritization table**
   - Open `/workspaces/CodeVald-AIProject/CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md` (lives in CodeValdCross — platform-wide)
   - Find the row for the selected task and change its Status to `🚀 In Progress`
   - Save the file

3. **Read the specification**
   - Re-read the relevant FRs in `documentation/1-SoftwareRequirements/requirements.md`
   - Re-read the corresponding section in `documentation/2-SoftwareDesignAndArchitecture/`
   - Read the task spec in `documentation/3-SofwareDevelopment/mvp-details/{topic-file}.md`
   - Understand how the task fits the layered design (gRPC server → `AIManager` → `entitygraph` storage / `LLMClient` / `CrossPublisher`)
   - Note any LLM, run-lifecycle, or schema constraints

4. **Create feature branch from `main`**
   ```bash
   cd /workspaces/CodeVald-AIProject/CodeValdAI
   git checkout main
   git pull origin main
   git checkout -b feature/AI-XXX_description
   ```
   Branch naming: `feature/AI-XXX_description` (lowercase with underscores)

5. **Read project guidelines**
   - Review `.github/instructions/rules.instructions.md`
   - Key rules: interface-first, no direct LLM SDK imports in domain code, no `grpc.Dial` inside `AIManager`, typed `AgentRunStatus` transitions, context propagation, godoc on all exports

6. **Create a todo list**
   - Break the task into actionable steps
   - Use the manage_todo_list tool to track progress
   - Mark items in-progress and completed as you go

## Pre-Implementation Checklist

Before starting:
- [ ] Relevant FRs and architecture sections re-read
- [ ] Feature branch created: `feature/AI-XXX_description`
- [ ] Existing files checked — no duplicate types in `models.go` or `errors.go`
- [ ] Understood which file(s) to modify (`ai.go`, `models.go`, `errors.go`, `internal/server/`, `internal/registrar/`, `storage/arangodb/`)
- [ ] Todo list created for this task

## Development Standards

- **No direct LLM SDK imports** in domain code — go through the `LLMClient` interface
- **No `grpc.Dial` in `AIManager`** — Cross publishes go through the injected `CrossPublisher`
- **No raw AQL or ArangoDB driver calls** outside `storage/arangodb/`
- **Run statuses are typed** — use `AgentRunStatus` constants from `models.go`
- **Every exported symbol** must have a godoc comment
- **Every exported method** takes `context.Context` as the first argument
- **Cross publish failures must NOT fail the originating operation** — log and continue
- **Never log API keys or full LLM payloads** — log run/agent IDs, status, token counts only

## Git Workflow

```bash
# Create feature branch
git checkout -b feature/AI-XXX_description

# Regular commits during development
git add .
git commit -m "AI-XXX: Descriptive message"

# Build validation before merge
go build ./...           # must succeed
go test -v -race ./...   # must pass
go vet ./...             # must show 0 issues
golangci-lint run ./...  # must pass

# Merge when complete
git checkout main
git merge feature/AI-XXX_description --no-ff
git branch -d feature/AI-XXX_description
```

## Success Criteria

- ✅ Relevant FR(s) and architecture doc reviewed
- ✅ Feature branch created from `main`
- ✅ Todo list created with implementation steps
- ✅ Ready to implement following service design rules
