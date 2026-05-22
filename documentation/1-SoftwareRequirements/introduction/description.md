**CodeValdAI** is the LLM execution engine for the CodeVald platform. It maintains a persistent registry of AI agents — each with its own model binding, system prompt, and provider config (Anthropic, OpenAI, or HuggingFace) — and runs a structured two-phase execution model: first the agent tells you what inputs it needs, then you fill them in and fire. Every run is tracked from `pending_intake` through `completed` or `failed`, with full token usage recorded and completion events published to the platform bus.

Part of the **CodeVald** platform — infrastructure for AI agents that reason, execute, and produce auditable output.

GitHub: https://github.com/aosanya/CodeValdAI
