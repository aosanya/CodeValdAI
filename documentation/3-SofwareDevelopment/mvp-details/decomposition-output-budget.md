# CodeValdAI — Decomposition Output Budget & Pre-Decomposition Splitting

> Part of the mvp-details. Index: [README.md](README.md)
>
> Companion to [task-decomposition.md](task-decomposition.md) — the existing
> decompose-into-todos mechanism. This doc proposes a planner layer in front
> of that mechanism.

| Field | Value |
|---|---|
| Status | Draft |
| Date | 2026-06-04 |
| Source finding | QA scenario 11 retry pass (2026-06-03T21:56–2026-06-04T18:11 UTC) — MVP-SF-002 (Auth: registration + login + JWT + router guard + farm creation) failed 4 consecutive decomposition attempts even after agent `max_tokens` was bumped from default 8192 to 16384 |
| Owner | CodeValdAI (decomposition path); CodeValdWork (task-graph schema for parent/child); CodeValdImplementations (agency.json planner agent + work plan) |

---

## 1. Overview

Reasoning models such as DeepSeek-V4-Pro can consume the entire output budget on `<think>` chain-of-thought before emitting the required fenced `actions` block. On complex multi-file tasks the LLM never produces an actions block, the `AgentRun` fails with `no actions block in LLM output`, and the parent task either burns through the retry ladder or sits IN_PROGRESS indefinitely.

The existing decomposition guards (`RULE FILE-SIZE-CAP`, `RULE ONE-FILE-PER-TODO`, etc.) cap per-todo cost but do not cap the *number* of todos one decomposition must produce. A 12-file task → 12 todos in one actions block → reasoning runaway → failure.

This doc proposes a **unified planner-as-first-step** architecture: every task assignment first invokes a small planner agent that decides whether to **split** the task into child Task entities or **decompose** it into todos. Pub/sub is the commit mechanism — the planner emits exactly one tiny fenced action declaring its decision, and the rest of the system reacts.

---

## 2. Current dispatch budget (as measured)

Plumbing chain that determines per-LLM-call token caps:

```
agency.json max_tokens  ┐
                        ├─► (only at ImportDraft if agent record didn't exist yet)
DB Agent.MaxTokens      ┴─► maxTokensOrDefault(n)  →  LLM body max_tokens
                                 (defaults to 8192 when n == 0)

agency.json session_max_tokens ┐
                               ├─► (same import constraint)
DB Agent.SessionMaxTokens      ┘
       coalesce with WorkPlan.wp_session_max_tokens
                  │
                  ▼
       SessionLimits.MaxTokens
                  │
                  ▼  execute.go stream-cancel when tokenCount >= cap
```

Critical gap surfaced this session: **`ImportDraft` does not update existing AI Agent entities.** When agency.json values for `max_tokens` / `session_max_tokens` are added later, existing agents retain whatever they had at first-create (often `0`, falling back to the 8192 default). Recovery is a `PUT /ai/{agency}/agents/{id}` carrying the new caps.

Measured behaviour on MVP-SF-002 with `max_tokens = 8192` default:
- `inputTokens = 3490`, `outputTokens = 6508`, errorMessage `no actions block in LLM output`
- LLM output stopped mid-sentence: `…But it might a…`
- Three retries (auto-dispatcher) produced effectively identical failures.

After bumping to `max_tokens = 16384`: a fourth retry also failed with the same error — increasing the cap merely lets the `<think>` block grow further; the model still does not commit to an actions block before stopping.

---

## 3. Proposed architecture — unified planner

### 3.1 Event flow

```
work.task.assigned
        │
        ▼
planner-assigned-handler   (planner agent — runs on EVERY task)
        │
        │  LLM emits ONE fenced action: ONE of —
        ▼
        ├─►  ai.task.split        (children + depends_on)
        │             │
        │             ▼
        │      split-handler (CodeValdAI or CodeValdWork)
        │             ├─ creates N child Task entities
        │             ├─ writes parent_task_id edges
        │             ├─ writes child depends_on edges (planner output)
        │             └─ parent → TASK_STATUS_SPLIT
        │                Children: TASK_STATUS_PENDING; next-task picks them
        │                up in topo order, each one fires work.task.assigned
        │                → planner runs again on each child (recursive)
        │
        └─►  ai.task.decompose    (signal-only — no children)
                      │
                      ▼
              developer-assigned-handler   (existing decomposition path)
                      │
                      └─ emits ai.task.todo  (today's mechanism, unchanged)
```

The current `developer-assigned-handler` work plan keeps its existing implementation; its **trigger topic changes** from `work.task.assigned` to `ai.task.decompose`. The planner becomes the new `work.task.assigned` subscriber.

### 3.2 Why pub/sub is the commit mechanism

The planner's output is one fenced action carrying the decision. If the LLM exhausts its budget before emitting *any* fenced action, the `AgentRun` fails with the existing `no actions block in LLM output` error, which surfaces to the AI Failure Reviewer — the recovery loop already handles this. No new failure path is introduced.

Because the planner's output is small (one action plus optionally a short children list), the realistic reasoning runaway risk is bounded: even a reasoning-heavy model has to spend most of its budget on `<think>` before it would run out, and the actions block itself is ~300–800 tokens.

### 3.3 Recursion

A child task that is *itself* too big to decompose still works. Its `work.task.assigned` fires the planner, planner picks `split` again, grandchildren get created. Eventually every leaf is small enough that the planner picks `decompose`. Recursion is bounded by the planner's own judgement; pathological cases bottom out at one-file tasks.

---

## 4. Data model

### 4.1 New Task fields / edges (CodeValdWork side)

| Field | Type | Purpose |
|---|---|---|
| `parent_task_id` | string (edge ref) | Set on every child Task. Null/empty on root tasks. |
| `TASK_STATUS_SPLIT` | new enum value | Terminal-for-dispatch state on parent. `next-task` skips SPLIT tasks. `maybeCompleteParentTask`-equivalent roll-up logic transitions SPLIT → COMPLETED / FAILED based on child terminal states. |
| `depends_on` | existing edge | Reused unchanged. Planner declares child[i].depends_on = [child[j].temp_id, …] in its output; split-handler resolves temp ids to real Task ids when creating children. |

### 4.2 New event payloads

`ai.task.split` payload:
```json
{
  "task_id":         "<parent task id>",
  "workflow_run_id": "<run id>",
  "children": [
    {
      "temp_id":     "c1",
      "task_name":   "MVP-SF-002a",
      "title":       "Auth — data models",
      "description": "...",
      "role_name":   "Developer",
      "depends_on":  []
    },
    {
      "temp_id":     "c2",
      "task_name":   "MVP-SF-002b",
      "title":       "Auth — API client",
      "description": "...",
      "role_name":   "Developer",
      "depends_on":  ["c1"]
    }
  ]
}
```

`ai.task.decompose` payload (signal-only):
```json
{
  "task_id":         "<task id>",
  "workflow_run_id": "<run id>"
}
```

### 4.3 Roll-up — reuse `maybeCompleteParentTask` semantics

The same group-and-decide pattern fixed in [BUG-20260603-007](../../bug-details/BUG-20260603-007_maybe-complete-parent-task-counts-superseded-todos.md) for todos applies at the task level:

- A SPLIT parent's children are inspected after each child reaches a terminal state.
- All children COMPLETED / SKIPPED → parent → COMPLETED.
- Any child FAILED → parent → FAILED (with direction-driven recovery via BUG-20260603-006 fix).
- Cascade-cancel on parent direction `cancel` cancels remaining children.

---

## 5. Planner agent configuration

| Field | Recommended |
|---|---|
| `code` | `task-planner` |
| `provider_code` | Same provider as developer (huggingface DeepSeek-V4-Pro) initially. Smaller / non-reasoning model is a tuning lever once the loop is proven. |
| `system_prompt` | Tight: "You decide whether a task should be SPLIT into smaller tasks or DECOMPOSED into todos. Emit exactly one fenced actions block. No prose outside the block." Plus the split/decompose schema. |
| `max_tokens` | 4096 (output is small; this also limits runaway) |
| `session_max_tokens` | 4096 |
| `temperature` | 0.3 (more deterministic than the developer at 0.7) |

`session_max_seconds` and `session_max_sessions` mirror the developer's defaults.

---

## 6. Token-cost analysis

Baselines from this session's measurements:

| Scenario | Today | With unified planner |
|---|---|---|
| Simple task (succeeds first try, ~8 todos) | 1 developer decomp (≈10K) + 8 impl todos (≈28K) = **~38K** | 1 planner (≈3K) + 1 developer decomp (≈10K) + 8 impl todos (≈28K) = **~41K** (+8%) |
| Complex task (today: 4 failed retries, stuck) | 4 × failed decomp (≈40K wasted, task not done) | 1 planner (≈3K) + 3 child planners (≈9K) + 3 child decomps (≈18K) + impl todos (≈28K) = **~58K, completes** |
| Pathological (2 split levels) | impossible | bounded by planner self-selection, ≤2× single-split cost |

**Break-even:** the planner is cheaper than the existing failure loop after ~1.5 wasted decomposition retries. Currently complex tasks are observed to fail 3–4 times before any recovery (and MVP-SF-002 has not recovered at all after 4 retries). The unified planner is net-cheaper *today*, not just in steady state.

---

## 7. Migration

1. **Add `task-planner` agent** to `agency.json ai_config.agents`. Reimport idempotently.
2. **Add `planner-assigned-handler` work plan** triggered on `work.task.assigned`. Bind to `task-planner`.
3. **Change `developer-assigned-handler` trigger** from `work.task.assigned` to `ai.task.decompose`. Reimport.
4. **Add CodeValdAI/CodeValdWork support** for emitting `ai.task.split` / `ai.task.decompose` topics in the action catalogue.
5. **Add CodeValdWork support** for `ai.task.split` consumption — split-handler creates children with parent_task_id and depends_on edges, transitions parent to `TASK_STATUS_SPLIT`.
6. **Add `TASK_STATUS_SPLIT` to the Task state machine** with the roll-up transitions described in §4.3.

Migration is staged — each step lands behind the next. The agency.json reimport pattern means rollback is one revert + reimport.

---

## 8. Open gaps

These are explicitly deferred for follow-up work; do not block on them.

- **Planner-model choice.** Initial guidance is "same model as developer". A non-reasoning model (Sonnet-Haiku class) is cheaper and likely sufficient for the decision-and-children-list output shape. Needs an A/B once the loop is wired.
- **What if planner picks `split` but emits zero children?** Treat as `no actions block` failure → AI Failure Reviewer. Document explicitly.
- **What if planner picks `split` with cyclic depends_on?** Split-handler validates and rejects; treat as planner failure.
- **`ai.task.split` action catalogue entry.** Need entry in the canonical action catalogue per [action-protocol.md](action-protocol.md) so the dispatcher accepts the topic.
- **TASK_STATUS_SPLIT in CodeValdWorkFrontend.** UI must render a SPLIT parent as a tree with child status aggregation (not as a stale IN_PROGRESS).
- **`ImportDraft` does not update existing AI Agent entities.** Surfaced this session when agency.json `max_tokens` bumps were silently ignored. File as a separate Agency bug; the planner work above is independent but benefits from the fix.

---

## 9. References

- [task-decomposition.md](task-decomposition.md) — the existing decompose-into-todos path that the planner's `decompose` branch invokes unchanged.
- [run-execution.md](run-execution.md) — `SessionLimits.MaxTokens` enforcement.
- [action-protocol.md](action-protocol.md) — fenced actions block contract; new topics need entries here.
- [llm-client/README.md](llm-client/README.md) — `max_tokens` plumbing into provider HTTP bodies.
- [BUG-20260603-007](../../bug-details/BUG-20260603-007_maybe-complete-parent-task-counts-superseded-todos.md) — `latestDecomposition` pattern that the parent-task roll-up should mirror.
- [BUG-20260603-006](../../bug-details/BUG-20260603-006_task-state-machine-blocks-direction-retry.md) — `failed → in_progress` rule that direction-driven recovery on SPLIT parents will rely on.
