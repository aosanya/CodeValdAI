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
| [BUG-20260603-002](bug-details/BUG-20260603-002_inline-hardcoded-git-topic-strings.md) | Inline hardcoded git-domain topic strings in execute.go and event_receiver.go | Low | 📋 Open | FEAT-20260603-001, FEAT-20260603-002 (SharedLib) |
| [BUG-20260603-001](bug-details/BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md) | Decomposition run with no actions block falsely completes the task | High | 📋 Open | — |

---

### BUG-20260603-002 — Inline hardcoded git-domain topic strings in execute.go and event_receiver.go

**Severity:** Low
**Status:** 📋 Open

`execute.go` (lines 524, 555, 559) and `event_receiver.go` (line 77) compare incoming event topics against raw `"git.*"` string literals. Because CodeValdAI cannot import CodeValdGit, these strings are orphaned with no compile-time guard against drift. They will silently stop matching if CodeValdGit renames any of these topics.

**Root cause:** SharedLib has no canonical `Topic*` constants for any domain. Once [FEAT-20260603-001](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-001_eventbus-domain-constants.md) and [FEAT-20260603-002](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-002_migrate-topic-constants-to-sharedlib.md) land, replace literals with `eventbus.TopicGitFileWrite`, `eventbus.TopicGitFileWritten`, `eventbus.TopicGitBranchCreate`.

See [bug-details/BUG-20260603-002_inline-hardcoded-git-topic-strings.md](bug-details/BUG-20260603-002_inline-hardcoded-git-topic-strings.md) for full fix plan.

---

### BUG-20260603-001 — Decomposition run with no actions block falsely completes the task

**Severity:** High
**Status:** 📋 Open

When a decomposition run produces no actions block (token cap hit with all output inside `<think>`, model refusal, or parser failure), `ExecuteRun` still publishes `ai.task.completed`. CodeValdWork marks the task `COMPLETED` with zero todos. The task enters a terminal state with no work done and cannot be retried.

**Root cause (Failure A):** Model exhausted all output tokens inside `<think>` before emitting an actions block. Possible sign that `developer-agent.max_tokens` is still at the old 4096 limit (BUG-09-028 Phase 2 should have raised it to 16384 — verify container binary is current).

**Root cause (Failure B):** `ExecuteRun` does not gate `ai.task.completed` on the presence of actions. `dispatchActions` returning `nil` when no actions are found is treated as success.

**Fix:** In `execute.go`, after `dispatchActions` returns, check whether any actions were found. If none: publish `ai.run.failed` + `ai.task.failed` instead of `ai.task.completed`. This correctly routes the task into the failure-pipeline rather than silently completing it.

See [bug-details/BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md](bug-details/BUG-20260603-001_decomp-no-actions-block-falsely-completes-task.md) for full fix plan.
