# BUG-20260609-001 — Drop `ai.` domain prefix from published topic names (system-wide rename)

**Status:** 🚀 In Progress
**Severity:** High — paired with [CodeValdWork/BUG-20260609-001](../../../../CodeValdWork/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_work_domain_prefix.md); both must rename in the same release window or the dispatch graph splits down the middle
**Owner:** CodeValdAI (primary — `ai.task.*` family); coordinated paired item in CodeValdWork; trigger-topic updates land in [CodeValdImplementations/Agencies/utility-app-builder/agency.json](../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json)
**Estimated effort:** ~1 day (audit + rename + agency.json sweep; smaller than the Work side because CodeValdAI has fewer publish call sites)
**Source finding:** `/document_issues` run during scenario-12 setup on 2026-06-09 — companion to the CodeValdWork bug; see that bug for the full session context

---

## Problem

CodeValdAI publishes every domain event with an `ai.` prefix:

| Today's topic | Producer | New topic |
|---|---|---|
| `ai.task.started` | AgentRun begins (before LLM call) | `task.started` |
| `ai.task.completed` | AgentRun finishes, actions dispatched | `task.completed` |
| `ai.task.failed` | LLM error / timeout / no actions block | `task.failed` |
| `ai.task.split` | planner emits split routing event | `task.request-split` |
| `ai.task.decompose` | planner emits decompose routing event | `task.request-decompose` |
| `ai.task.todo` | developer agent emits per-todo entries during decomposition | `task.todo` |

The new direction — confirmed by user decision on 2026-06-09 during `/document_issues` — is to drop the `work.` / `ai.` / `git.` / `comm.` prefixes everywhere. Topic names become intent-keyed; producer is no longer part of the name.

Note that the planner-to-router event names also gain semantic clarity in the rename: `ai.task.split` (publisher-keyed) → `task.request-split` (intent-keyed: a planner is requesting a split). Same for `ai.task.decompose` → `task.request-decompose`.

This conflicts with the now-retired `feedback_domain_event_rule.md` auto-memory.

Until the rename lands:

- `flows_planning.json` step-1.1 trigger `task.request-split` / `task.request-decompose` never matches the real `ai.task.split` / `ai.task.decompose` publishes.
- `developer-assigned-handler` WorkPlan has `trigger_topic: ai.task.decompose` — won't match the new convention.
- Scenario-12 QA pubsub assertions for `task.request-decompose` / `task.todo` events all fail.

## Evidence

```text
$ grep -nc 'ai\.task\.' CodeValdAI/internal/ CodeValdAI/cmd/ -r 2>/dev/null | grep -v :0
# (multiple files emit ai.task.{started,completed,failed,split,decompose,todo})

$ curl -s "http://codevaldcross:8081/agency/utility-app-builder/work-plans" -u "..." \
    | python3 -c "
import sys, json
plans = json.load(sys.stdin).get('entities', [])
p = next(x for x in plans if x['properties'].get('code') == 'developer-assigned-handler')
print(f'trigger_topic = {p[\"properties\"][\"trigger_topic\"]}')
"
trigger_topic = ai.task.decompose                                     ← old prefix
```

## Root cause

Same as the Work bug — the Domain event ownership rule prefixed every emitted topic with the publisher's service domain. User retired the rule on 2026-06-09; producers and trigger_topics never followed.

## Fix plan (phased)

Mirrors the [CodeValdWork bug](../../../../CodeValdWork/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_work_domain_prefix.md). High-level:

### Phase 1 — SharedLib eventreceiver (upstream, shared)

Drop the `<service-domain>.` prefix from auto-constructed topic names. Dual-emit shim for a transition window. See the Work bug for details — this is the same upstream change.

### Phase 2 — CodeValdAI rename

Files to audit (non-exhaustive):

- `internal/dispatcher/` — RACI / WorkPlan match emit paths
- `internal/runner/` — AgentRun lifecycle (`ai.task.started/completed/failed`)
- `internal/planner/` — planner emit paths (`ai.task.split/decompose`)
- `internal/handlers/decomp/` — `ai.task.todo` emit
- `actions.go` / `dispatchActions.go` — any hard-coded topic literals
- `topics.go` (if it exists) — central constants table

Replace per the rename table. Update unit tests.

### Phase 3 — agency.json trigger_topics

In [agency.json](../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json), update every WorkPlan whose `trigger_topic` is in the `ai.*` rename table:

- `developer-assigned-handler`: `ai.task.decompose` → `task.request-decompose`
- (audit the rest — any `task-split-handler` once it ships from [CodeValdAgency/FEAT-20260609-001](../../../../CodeValdAgency/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260609-001_task_split_handler_workplan.md) should land already-named with the new convention)

Reimport via the auto-promote path ([FEAT-20260609-003](../../../../CodeValdAgency/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260609-003_auto_draft_on_import.md)).

### Phase 4 — Scenario-12 QA verification

After Phases 1–3 land, scenario-12 QA assertions (already using un-prefixed names) match real events. Smoke test.

### Phase 5 — Retire the auto-memory

Done as part of the Work bug — single retirement covers both. See [CodeValdWork/BUG-20260609-001 Phase 5](../../../../CodeValdWork/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_work_domain_prefix.md).

## Verification

- [ ] All unit tests in CodeValdAI pass with the new topic constants.
- [ ] Scenario-12 pubsub assertions for `task.request-decompose`, `task.request-split`, and `task.todo` match real events on a live deploy.
- [ ] No consumer is subscribed to `ai.*` after the dual-emit window closes.
- [ ] Companion Work rename landed in the same release.

## Dependencies

- **Hard depends on** SharedLib Phase 1 (dual-emit shim) — same upstream as the Work bug.
- **Paired with** [CodeValdWork/BUG-20260609-001](../../../../CodeValdWork/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_work_domain_prefix.md). Roll out together.
- **Soft depends on** [CodeValdAgency/FEAT-20260609-003](../../../../CodeValdAgency/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260609-003_auto_draft_on_import.md) for clean reimport during rollout.

## Risks

- Half-renamed graph (if Phase 2 lands before Phase 3) — same mitigation as the Work bug: SharedLib dual-emit covers the transition.
- The planner-emitted `task.request-split` / `task.request-decompose` names are semantic upgrades over the old `ai.task.split` / `ai.task.decompose`. Verify all consumers understand the intent shift (they're requests, not statements) — particularly `task-split-handler` once it ships.
