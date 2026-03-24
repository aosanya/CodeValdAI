```markdown
# CodeValdAI — Problem Definition

## Context

The CodeVald platform orchestrates AI agents to complete work on behalf of an Agency.
Agencies define their structure through Goals, Workflows, and WorkItems (owned by CodeValdAgency).
Tasks are dispatched and tracked by CodeValdWork.

But **who actually calls the LLM, with what context, and how are the results collected?**
That gap is CodeValdAI's responsibility.

---

## Problem

Without a dedicated AI execution service:

- **No persistent agent registry** — model choice, system prompts, and temperature settings are
  scattered across callers, inconsistently applied, and impossible to audit or version.
- **No structured input negotiation** — callers must guess what information an agent needs before
  a run starts. Runs fail mid-way because required context was missing.
- **No run history** — there is no record of what was sent to the LLM, what came back, how many
  tokens were used, or when the run completed.
- **No provider abstraction** — switching from one LLM provider to another requires changes across
  multiple services.

---

## Solution

CodeValdAI introduces two first-class concepts:

### 1. Agent
A named, versioned configuration for an LLM interaction:
- Which **provider** and **model** to use (e.g. Anthropic / `claude-3-5-sonnet-20241022`)
- A **system prompt** that sets the agent's persona and constraints
- Default **parameters** (temperature, max_tokens)

Agents are persisted as graph entities in ArangoDB, scoped to an Agency.

### 2. AgentRun
A two-phase execution record:

**Phase 1 — Intake**: The caller submits `agent_id`, `workflow_id`, and `instructions`.
CodeValdAI asks the LLM: *"given this workflow and these instructions, what inputs do you need?"*
The LLM returns a structured field schema. CodeValdAI stores this as `RunField` entities linked
to the `AgentRun` and returns them to the caller.

**Phase 2 — Execute**: The caller fills the fields and submits them back (referencing the same `run_id`).
CodeValdAI calls the LLM with the full context — agent system prompt, workflow, instructions, and
filled inputs — stores the output, and publishes a completion event.

---

## Boundaries

| Concern | Owner |
|---|---|
| Agency structure, Goals, Workflows | CodeValdAgency |
| Task creation and status | CodeValdWork |
| Artifact storage (LLM output files) | CodeValdGit |
| Communication channels | CodeValdComm |
| Digital twin graph | CodeValdDT |
| **AI agent registry + LLM execution** | **CodeValdAI** |
```
