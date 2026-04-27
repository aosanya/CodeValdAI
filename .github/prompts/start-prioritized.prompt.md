---
agent: agent
---

# Start Prioritized Task

> ⚠️ **Before starting**, run the following documentation hygiene check across
> **every repo you are about to touch**, then fix any hits before writing a

> ⚠️ **Before starting**, after picking a task, confirm no feature branch is currently open in **the target repo** for that task.
> If one exists, finish and merge it first in that repo only (see the **Finish** section at the bottom). Branches in other repos do not block you.

The **authoritative task list** for the whole platform is:
`/workspaces/CodeVald-AIProject/CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md`

All task selection, status updates, and done-tracking flow through that file.

---

## Step 1 — Select the Next Task

1. Read `prioritization.md` (path above).
2. Pick the **first row** whose Status is `📋 Not Started` or `🚀 In Progress` — the file is already in priority order, no sorting needed.
   - Skip rows whose Status is `⏸️ Blocked` .
3. Note the **Task ID**, **Title**, **Service**, and **Depends On** values from that row.
4. ⚠️ Do not overthink!!!
   - Create Feature branch
   - Update status in prioritization.md and respective mvp.md
   - Dive into the task and do it! WE have never deployed the applications, do not be afraid of breaking things, especially tests
---

## Step 2 — Identify the Target Repository

Use the Task ID prefix to look up the correct repo, branch prefix, and validation stack:

| Task ID prefix | Service | Repository path | Branch prefix | Validation |
|---|---|---|---|---|
| `MVP-AI-` | CodeValdAI | `/workspaces/CodeVald-AIProject/CodeValdAI` | `feature/AI-NNN_` | Go |
| `SHAREDLIB-` | CodeValdSharedLib | `/workspaces/CodeVald-AIProject/CodeValdSharedLib` | `feature/SHAREDLIB-NNN_` | Go |
| `CROSS-` | CodeValdCross | `/workspaces/CodeVald-AIProject/CodeValdCross` | `feature/CROSS-NNN_` | Go |
| `MVP-GIT-` | CodeValdGit | `/workspaces/CodeVald-AIProject/CodeValdGit` | `feature/GIT-NNN_` | Go |
| `MVP-WORK-` | CodeValdWork | `/workspaces/CodeVald-AIProject/CodeValdWork` | `feature/WORK-NNN_` | Go |
| `MVP-AGENCY-` | CodeValdAgency | `/workspaces/CodeVald-AIProject/CodeValdAgency` | `feature/AGENCY-NNN_` | Go |
| `MVP-DT-` | CodeValdDT | `/workspaces/CodeVald-AIProject/CodeValdDT` | `feature/DT-NNN_` | Go |
| `MVP-COMM-` | CodeValdComm | `/workspaces/CodeVald-AIProject/CodeValdComm` | `feature/COMM-NNN_` | Go |
| `MVP-HI-` | CodeValdHi | `/workspaces/CodeVald-AIProject/CodeValdHi` | `feature/HI-NNN_` | Flutter |

> `NNN` is the numeric portion of the Task ID.
> Examples:
> - `MVP-AI-001` → `feature/AI-001_module-scaffolding`
> - `MVP-DT-002` → `feature/DT-002_arangodb-backend`
> - `MVP-AGENCY-007` → `feature/AGENCY-007_agency-publishing`
> - `SHAREDLIB-010` → `feature/SHAREDLIB-010_entitygraph-package`

---

## Step 3 — Create the Feature Branch

Run in the **target repo** — not in CodeValdCross:

```bash
cd {REPO_PATH}
git checkout main
git pull origin main
git checkout -b feature/{PREFIX}-{NNN}_{short-description}
```

- Description: lowercase, words separated by hyphens.
- **Never commit directly to `main`.**
- The branch must exist before any file is touched.

---

## Step 4 — Update Status

**In `prioritization.md`** (always lives in CodeValdCross regardless of which service you are working in):
- Change the selected task's Status from `📋 Not Started` → `🚀 In Progress`.

**In the service's own `mvp.md`**:
- Change the same task's Status to `🚀 In Progress`.

---

## Step 5 — Read Service Context

Inside the **target repo**, read these files before writing any code:

1. `.github/instructions/rules.instructions.md` — interface-first design, error types, file size limits, naming, concurrency rules.
2. `documentation/2-SoftwareDesignAndArchitecture/architecture.md` — interface contracts, data models, storage schema, gRPC definitions.
3. `documentation/3-SofwareDevelopment/mvp.md` — confirm every task listed in **Depends On** is ✅ complete.

**Always treat CodeValdAgency as the canonical reference implementation.**
Read `/workspaces/CodeVald-AIProject/CodeValdAgency/.github/instructions/rules.instructions.md`
and mirror its patterns (file layout, injection style, heartbeat registrar, error mapping, etc.)
when scaffolding or implementing any Go gRPC service.

**SharedLib extraction rule (applies throughout the entire task):**
> Whenever you encounter infrastructure code that is — or could soon be — used by more than one
> service (e.g. registration helpers, ArangoDB bootstrap, gRPC server utilities, shared types),
> **stop and flag it explicitly**: describe what the candidate is, which services would benefit,
> and ask the user how to proceed before continuing.
> Never silently copy code across services; instead surface the opportunity for SharedLib extraction.

**For CodeValdAI tasks specifically**, also read:
- `/workspaces/CodeVald-AIProject/CodeValdAI/documentation/` — full architecture split files
- `/workspaces/CodeVald-AIProject/CodeValdAI/documentation/3-SofwareDevelopment/mvp.md` — 16-task breakdown with acceptance tests and implementation walkthroughs
- `/workspaces/CodeVald-AIProject/CodeValdAI/documentation/3-SofwareDevelopment/mvp-details/` — per-task specs (scaffolding, llm-client, agent-management, run-intake, run-execution)

---

## Step 6 — Pre-Implementation Checklist

- [ ] Task selected from `prioritization.md`; all `Depends On` items are ✅ complete
- [ ] Target repo identified from the Task ID prefix lookup table
- [ ] Feature branch created in the **target repo**: `feature/{PREFIX}-{NNN}_{description}`
- [ ] `prioritization.md` updated to `🚀 In Progress`
- [ ] Service `mvp.md` updated to `🚀 In Progress`
- [ ] `rules.instructions.md` and `architecture.md` read for the target service
- [ ] Checked `models.go` / `errors.go` / `types.go` — no duplicate types
- [ ] Todo list created with actionable implementation steps

---

## Step 7 — Implement

- Use the `manage_todo_list` tool to track each step.
- Mark items in-progress and completed as you go.
- Commit regularly: `git add . && git commit -m "{TASK-ID}: Descriptive message"`
- Keep files under 500 lines; functions under 50 lines.
- Every exported symbol gets a godoc comment; every exported method takes `context.Context` first.

---

## Finish — Validate, Merge, and Close the Branch

Run all checks **in the target repo** before merging.

### Go services (all except CodeValdHi)

```bash
cd {REPO_PATH}

go build ./...           # must succeed
go vet ./...             # must show 0 issues
go test -v -race ./...   # must pass
golangci-lint run ./...  # must pass

# If proto files changed:
buf lint
buf generate && git diff --exit-code gen/
```

### Flutter — CodeValdHi only

```bash
cd /workspaces/CodeVald-AIProject/CodeValdHi

flutter analyze                                        # must show 0 issues
flutter test                                           # must pass
dart format --set-exit-if-changed lib/ test/           # must show 0 changes
```

### Merge and delete — in the target repo

```bash
cd {REPO_PATH}
git checkout main
git merge feature/{PREFIX}-{NNN}_{description} --no-ff
git branch -d feature/{PREFIX}-{NNN}_{description}
```

> ⚠️ Do **not** merge in CodeValdCross unless the task itself belongs to CodeValdCross.
> Each service owns its own `main` branch and its own feature branches.

### Update documentation after merge

1. **`prioritization.md`** (CodeValdCross) — remove the completed task's row entirely.
   Completed tasks are not shown in the active table.
2. **Service `mvp.md`** — change the task status to `✅ Done`.
3. **Service `mvp_done.md`** — add a row for the completed task with today's date.
4. If new architecture artefacts were introduced (topics, gRPC methods, flows, ArangoDB collections),
   update `documentation/2-SoftwareDesignAndArchitecture/architecture.md` in the target service.
