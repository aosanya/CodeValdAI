# CodeValdAI — Active Bug Backlog

## Overview

Bugs in scope for CodeValdAI. Mirrors the `mvp.md` / `mvp_done.md` / `mvp-details/` layout used for feature work.

- **Fixed bugs**: see [`bugs_done.md`](bugs_done.md)
- **Per-bug detail**: see [`bug-details/`](bug-details/)
- **Master cross-service queue**: [`../../../CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md`](../../../CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md)

## Workflow

### Completion Process (MANDATORY)
1. Implement and validate (`go build ./...`, `go vet ./...`, `go test -race ./...`)
2. Move the bug row from this file to `bugs_done.md`
3. Update the detail file's Status header to `✅ Fixed (YYYY-MM-DD)` and cite the commit / branch
4. Strike-through + ✅ the entry on the master prioritization.md
5. Merge feature branch to main

### Status Legend
- 📋 **Open** — not yet started or in triage
- 🚀 **In Progress** — actively being worked
- ⏸️ **Blocked** — waiting on a dependency
- ✅ **Fixed** — moved to `bugs_done.md` (do not list here)

---

## Active Bugs

| Bug ID | Title | Severity | Status | Depends On |
|--------|-------|----------|--------|------------|
| [BUG-20260609-001](bug-details/BUG-20260609-001_drop_ai_domain_prefix.md) | Drop `ai.` domain prefix from published topic names (system-wide rename; paired with CodeValdWork) | High | 📋 Open | SharedLib dual-emit shim; paired CodeValdWork BUG-20260609-001 |
| [BUG-20260603-001](bug-details/BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md) | Decomposition run with no actions block falsely completes the task | High | 📋 Open | — |

---

### BUG-20260609-001 — Drop `ai.` domain prefix from published topic names (system-wide rename)

**Severity:** High — paired with the CodeValdWork rename; both must land in the same release window
**Status:** 📋 Open

CodeValdAI publishes on `ai.task.started`, `ai.task.completed`, `ai.task.failed`, `ai.task.split`, `ai.task.decompose`, `ai.task.todo`. The rename drops the `ai.` prefix and shifts planner routing events from publisher-keyed to intent-keyed: `ai.task.split` → `task.request-split`, `ai.task.decompose` → `task.request-decompose`, `ai.task.todo` → `task.todo`. Phased rollout via SharedLib eventreceiver dual-emit shim, CodeValdAI publisher rename, agency.json `trigger_topic` sweep. Paired with [CodeValdWork/BUG-20260609-001](../../../CodeValdWork/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_work_domain_prefix.md) for the `work.*` family.

See [bug-details/BUG-20260609-001](bug-details/BUG-20260609-001_drop_ai_domain_prefix.md) for the full phased plan and rename table.

---

### BUG-20260603-001 — Decomposition run with no actions block falsely completes the task

**Severity:** High
**Status:** 📋 Open

When a decomposition run produces no actions block (token cap hit with all output inside `<think>`, model refusal, or parser failure), `ExecuteRun` still publishes `ai.task.completed`. CodeValdWork marks the task `COMPLETED` with zero todos. The task enters a terminal state with no work done and cannot be retried.

**Root cause (Failure A):** Model exhausted all output tokens inside `<think>` before emitting an actions block. Possible sign that `developer-agent.max_tokens` is still at the old 4096 limit (BUG-09-028 Phase 2 should have raised it to 16384 — verify container binary is current).

**Root cause (Failure B):** `ExecuteRun` does not gate `ai.task.completed` on the presence of actions. `dispatchActions` returning `nil` when no actions are found is treated as success.

**Fix:** In `execute.go`, after `dispatchActions` returns, check whether any actions were found. If none: publish `ai.run.failed` + `ai.task.failed` instead of `ai.task.completed`. This correctly routes the task into the failure-pipeline rather than silently completing it.

See [bug-details/BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md](bug-details/BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md) for full fix plan.
