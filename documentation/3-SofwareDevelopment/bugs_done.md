# CodeValdAI — Fixed Bugs

Bugs marked Fixed are removed from `bugs.md` and recorded here with their resolution date and the commit / branch that landed the fix.

| Bug ID | Title | Severity | Fixed Date | Commit / Branch | Detail |
|--------|-------|----------|------------|-----------------|--------|
| BUG-20260609-001 | Drop `ai.` domain prefix from published topic names (system-wide rename — SharedLib + AI + Work + Implementations + Cross) | High | 2026-06-10 | main | [bug-details/BUG-20260609-001](bug-details/BUG-20260609-001_drop_ai_domain_prefix.md) |
| BUG-20260603-002 | Inline hardcoded git-domain topic strings in `execute.go` and `event_receiver.go` | Low | 2026-06-08 | main | [bug-details/BUG-20260603-002](bug-details/BUG-20260603-002_inline-hardcoded-git-topic-strings.md) |
| BUG-20260603-003 | Todo returning `[]` (empty actions) is marked AGENT_RUN_STATUS_FAILED; cascades to task/run FAILED | High | 2026-06-03 | main (104acd7) | [bug-details/BUG-20260603-003](bug-details/BUG-20260603-003_empty-actions-todo-marked-failed.md) |
| BUG-09-028 | `parseActions` drops complete in-`<think>` actions block when post-think block is truncated | High | 2026-06-02 | main | [bug-details/BUG-09-028_actions_parser_drops_complete_block_when_post_think_truncated.md](bug-details/BUG-09-028_actions_parser_drops_complete_block_when_post_think_truncated.md) |
