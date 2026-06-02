# BUG-09-028 — `parseActions` drops the complete in-`<think>` actions block when the post-think block is truncated by the token cap

**Status:** ✅ Fixed (2026-06-02) — Phase 1 (parser fallback), Phase 2 (config bump to 16384), and Phase 3 (cap-hit warning) all landed on main
**Severity:** High — every decomp run that exceeds the agent's `max_tokens` cap silently produces no todos; the pipeline halts at Work-2 with no error event
**Owner:** CodeValdAI (parser) + ops (config)
**Source finding:** Hit during the 09 Part-G QA run on 2026-06-02 (MVP-SF-001, decomp run `9c17d013-...`)

## Resolution

- Phase 1 — [`parseActions`](../../../actions.go) now captures `<think>` contents before stripping; when the post-think actions block has an opening fence with no close, it falls back to a complete in-`<think>` block and logs `parseActions: post-think actions block truncated; falling back to in-think block`. Covered by `TestParseActions_FallsBackToInThinkWhenPostThinkTruncated` and four supporting tests in [actions_test.go](../../../actions_test.go).
- Phase 2 — `developer-agent.max_tokens` raised from 8192 → 16384 in [`CodeValdImplementations/Agencies/utility-app-builder/agency.json`](../../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json).
- Phase 3 — [`ExecuteRun`](../../../execute.go) emits a `WARN: output_tokens hit cap` log line at LLM-completion time when `output_tokens >= max_tokens`, so cap saturation is visible immediately instead of surfacing later as a parse failure.

---

## Problem

DeepSeek (and similar reasoning models on Novita) emit output of the form:

```
<think>
... long reasoning ...
```actions
[ ... complete, valid 10-todo JSON ... ]
```
... more reasoning ...
</think>

```actions
[ ... duplicate actions block ... ]
```        ← model never reaches this closing fence; truncated at max_tokens
```

`parseActions` ([CodeValdAI/actions.go:42](../../../actions.go)):

1. Calls `stripThinkBlocks` ([CodeValdAI/intake.go:183](../../../intake.go)), which removes everything between `<think>` and `</think>`. The valid 10-todo block dies with the rest of the reasoning.
2. Searches the remaining text for the first `` ```actions\n `` → finds the *unclosed* post-think block.
3. Looks for closing `` ``` ``, finds none, returns `actions block has opening fence but no closing` error.
4. `dispatchActions` logs the error and returns `hasSubtasks=false, emittedWrites=nil` — no `ai.todo.created` published, no todos created, run finishes "completed" with no follow-up.

Net effect: a 16-KB LLM output containing a perfectly valid action block is dropped and the pipeline silently halts.

## Evidence

```
$ grep -E "actions|malformed" docker compose logs codevaldai
codevaldai-1 | codevaldai: ExecuteRun ... llm ok: input_tokens=3298 output_tokens=4096 output_len=16220
codevaldai-1 | codevaldai: dispatchActions: malformed actions block: actions block has opening fence but no closing ```

$ python3 -c "
import json,re
d = json.load(open('/tmp/qa09-decomp.json'))
o = d['output']
print('fence count:', o.count('```actions'))           # 2
print('positions:', [m.start() for m in re.finditer(r'```', o)])  # [7614, 13215, 15143]
"
fence count: 2
positions: [7614, 13215, 15143]

# Position 7614  = first ```actions inside <think>
# Position 13215 = closing ``` for the in-think block (10 valid todos)
# Position 15143 = second ```actions after </think>
# (no fourth ``` because the model hit output_tokens=4096 mid-content)
```

The QA-side parser uses non-greedy regex `\`\`\`actions\n(.*?)\`\`\`` and *does* extract 10 todos from the same output — only the production dispatcher fails.

## Root cause

Two compounding factors:

1. **Parser fragility.** `stripThinkBlocks` is too aggressive. It assumes the post-think block is canonical, which is true *if* the model emits both. When the post-think block is truncated and the in-think block is complete, the canonical content is in the discarded reasoning.

2. **Token-cap default + agent config gap.** `maxTokensOrDefault(0) = 4096` ([CodeValdAI/dispatch.go:357](../../../dispatch.go#L357)). The developer-agent in the seeded utility-app-builder agency has `max_tokens=None`, so every decomp request goes out with `max_tokens=4096`. DeepSeek with `<think>` blocks typically burns 2–3 KB just reasoning; remaining budget is ~1.5 KB which is not enough for any non-trivial decomposition.

## Fix plan

### Phase 1 (parser, defensive) — owner: CodeValdAI

Update `parseActions` to:

1. Before stripping, save `<think>...</think>` contents.
2. Try parsing post-think actions block normally.
3. **If post-think has an open fence but no close, AND the saved think content contains a complete `` ```actions ``…`` ``` `` block, parse that one** and emit a warning log `parseActions: post-think actions block truncated; falling back to in-think block`.
4. Otherwise behave as today.

This keeps the original "post-think is canonical when complete" semantics while gracefully degrading on the very real truncation case.

### Phase 2 (config) — owner: utility-app-builder seed / Implementations

Set `max_tokens` on the developer-agent to at least `8192` (recommended `16384` to leave headroom for `<think>` overhead on multi-file decompositions). The seed lives in [`CodeValdImplementations/Agencies/utility-app-builder/agency.json`](../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json).

### Phase 3 (observability) — owner: CodeValdAI

When the LLM response has `output_tokens >= max_tokens`, log a warning at completion time so future truncations are obvious instead of surfacing 30 seconds later as a parse failure. Already nearly free — the log line at execute.go:179 has `MaxTokens` in scope; add a comparison and a `WARN: output_tokens hit cap` line.

## Verification

After Phase 1 + 2:

- Re-run Work-2 on a fresh `work.next.requested`. Either:
  - The post-think block fits in the new cap (Phase 2 alone unblocks the QA), or
  - The truncation re-occurs but the parser falls back to the in-think block (Phase 1 saves it).
- `ai.todo.created` events appear in PubSub (`curl .../events?topic=ai.todo.created` returns ≥1).
- `work.todo.dispatched` count equals decomp TODO_COUNT.
- The branch-creation todo fires `git.branch.create`; `feature/{TASK_NAME}-…` appears in `GET /git/{agency}/repositories/shared-farms/branches`.

## Workaround applied during QA run

For the 2026-06-02 QA run we applied Phase 2 only — set `max_tokens=16384` on the developer-agent in the running CodeValdAI instance. Phase 1 (parser) remains open as a follow-up.

## Dependencies

- None blocking. Phase 1 is a self-contained parser change with new unit tests.
- Phase 2 is a single config tweak that ops can apply without a code change.
