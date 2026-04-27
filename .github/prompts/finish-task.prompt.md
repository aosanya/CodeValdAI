---
agent: agent
---

# Finish Task

Follow the **mandatory task completion process** for CodeValdAI tasks:

## Task Completion Process (MANDATORY)

### Step 1: Validate the Implementation

Run all checks in order — **do not proceed to merge if any fail**:

```bash
cd /workspaces/CodeVald-AIProject/CodeValdAI

# 1. Build the binary
go build -o bin/codevaldai ./cmd/...

# 2. Static analysis
go vet ./...

# 3. Tests with race detector
go test -v -race ./...

# 4. Lint
golangci-lint run ./...

# 5. Protobuf check (if proto files changed)
buf lint
buf generate && git diff --exit-code gen/
```

### Step 2: Self-Review Checklist

Before marking the task complete:

- [ ] No direct LLM SDK imports in domain code (only inside the `LLMClient` impl)
- [ ] No `grpc.Dial` inside `AIManager` — Cross publishes go through `CrossPublisher`
- [ ] No raw AQL or ArangoDB driver calls outside `storage/arangodb/`
- [ ] All `AgentRunStatus` writes use the typed constants from `models.go`
- [ ] All exported symbols have godoc comments
- [ ] `context.Context` is the first argument of every exported method
- [ ] LLM calls propagate the caller's `ctx` and check `ctx.Err()` after returning
- [ ] Errors are wrapped with flow name and key IDs: `fmt.Errorf("ExecuteRun %s: %w", runID, err)`
- [ ] Cross publish failures are logged but do NOT fail the originating operation
- [ ] No API keys or full LLM payloads logged
- [ ] No files exceed 500 lines
- [ ] Tests added for new exported methods, with success and error cases
- [ ] `go vet ./...` shows 0 issues
- [ ] `go test -race ./...` passes

### Step 3: Update Documentation

- Update `documentation/3-SofwareDevelopment/mvp.md` — mark task as complete
- Update `documentation/3-SofwareDevelopment/mvp_done.md` — add row with today's date
- Update `/workspaces/CodeVald-AIProject/CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md` — remove the completed task's row (completed tasks are not shown in the prioritization table)
- If new Cross topics were introduced (`cross.ai.{agencyID}.…`), document them in `documentation/2-SoftwareDesignAndArchitecture/`
- If new entity types or edges were added, update `schema.go` AND the architecture doc together
- If new gRPC methods were added, update the proto and the architecture doc

### Step 4: Merge

```bash
git checkout main
git merge feature/AI-XXX_description --no-ff
git branch -d feature/AI-XXX_description
```

## Success Criteria

- ✅ All build, vet, test, and lint checks pass
- ✅ Self-review checklist complete
- ✅ Documentation updated
- ✅ Branch merged and deleted
