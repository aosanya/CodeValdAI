# BUG-20260603-002 (AI) — Inline hardcoded git-domain topic strings in `execute.go` and `event_receiver.go`

**Status:** 📋 Open
**Severity:** Low — functional today (strings match what CodeValdGit publishes), but will silently break if `git.*` topic names change, with no compile-time warning
**Owner:** CodeValdAI
**Estimated effort:** ~0.5 day (replace literals with SharedLib constants once FEAT-20260603-001 lands)
**Source finding:** Architecture audit 2026-06-03 — Rule 7b: hardcoded cross-domain string literals in production logic

## Problem

`CodeValdAI/execute.go` and `CodeValdAI/internal/server/event_receiver.go` compare incoming event topics against raw string literals from the `git.*` domain. Because CodeValdAI cannot import CodeValdGit directly (inter-service import is forbidden), and SharedLib does not yet define canonical topic constants, these strings are orphaned — there is nothing to keep them in sync with CodeValdGit's `events.go`.

## Evidence

```go
// CodeValdAI/execute.go:524
if a.Topic == "git.file.write" && run.ID != "" {

// CodeValdAI/execute.go:555
case "git.file.write":

// CodeValdAI/execute.go:559
case "git.branch.create":

// CodeValdAI/internal/server/event_receiver.go:77
if req.GetTopic() == "git.file.written" {
```

The corresponding canonical definitions in CodeValdGit:
```go
// CodeValdGit/events.go:55
TopicFileWrite   = "git.file.write"
// CodeValdGit/events.go:60
TopicFileWritten = "git.file.written"
// CodeValdGit/events.go:24
TopicBranchCreate = "git.branch.create"
```

## Root cause

SharedLib has no canonical topic constants for any domain (see [FEAT-20260603-001](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-001_eventbus-domain-constants.md)). The broader Topic migration ([FEAT-20260603-002](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-002_migrate-topic-constants-to-sharedlib.md)) will add canonical SharedLib topic constants for each domain so that cross-domain references like these can be type-safe.

## Fix plan

**Prerequisite**: [FEAT-20260603-001](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-001_eventbus-domain-constants.md) (Domain* constants) and [FEAT-20260603-002](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-002_migrate-topic-constants-to-sharedlib.md) (Topic constant migration) must land first — FEAT-20260603-002 will add `eventbus.TopicGitFileWrite`, `eventbus.TopicGitFileWritten`, `eventbus.TopicGitBranchCreate` to SharedLib's eventbus package.

Once those land, replace each inline string in CodeValdAI:

```go
// execute.go — before
if a.Topic == "git.file.write" && run.ID != "" {
// execute.go — after
if a.Topic == eventbus.TopicGitFileWrite && run.ID != "" {

// event_receiver.go — before
if req.GetTopic() == "git.file.written" {
// event_receiver.go — after
if req.GetTopic() == eventbus.TopicGitFileWritten {
```

Add `"github.com/aosanya/CodeValdSharedLib/eventbus"` to the import block in both files.

## Verification

```bash
grep -n '"git\.' /workspaces/CodeVald-AIProject/CodeValdAI/execute.go
grep -n '"git\.' /workspaces/CodeVald-AIProject/CodeValdAI/internal/server/event_receiver.go
# → no output
go build ./... && go vet ./...  # in CodeValdAI root
```

## Dependencies

- Blocked on: [FEAT-20260603-001](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-001_eventbus-domain-constants.md) — SharedLib Domain* constants
- Blocked on: [FEAT-20260603-002](../../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260603-002_migrate-topic-constants-to-sharedlib.md) — SharedLib must also export canonical Topic constants per domain
