```markdown
# CodeValdAI — Graph Topology & Schema

> Part of the split architecture. Index: [architecture.md](architecture.md)

---

## 1. Graph Topology

```
Agent ──has_run──► AgentRun ──has_field──► RunField
                             ──has_input──► RunInput
```

All nodes and edges live in two ArangoDB collections:
- **`ai_entities`** — document collection (Agent, AgentRun, RunField, RunInput)
- **`ai_relationships`** — **edge** collection (all relationship types)

---

## 2. Entity Types

| Type | Mutable | Properties | Notes |
|---|---|---|---|
| `Agent` | ✅ | `name`(req), `description`, `provider`(req), `model`(req), `system_prompt`(req), `temperature`, `max_tokens` | Root catalogue entry; one or many per agency |
| `AgentRun` | ✅ | `agent_id`(req), `workflow_id`, `instructions`(req), `status`(req), `output`, `error_message`, `input_tokens`, `output_tokens`, `started_at`, `completed_at` | Two-phase execution record |
| `RunField` | ❌ immutable | `fieldname`(req), `type`(req), `label`(req), `required`(req), `options`, `ordinality`(req) | Inferred by LLM during Intake; written once |
| `RunInput` | ❌ immutable | `fieldname`(req), `value`(req) | Submitted by caller during Execute; written once |

### `AgentRunStatus` values

| Value | Meaning |
|---|---|
| `pending_intake` | Run created at Intake; waiting for caller to submit inputs |
| `pending_execution` | Inputs received; queued for LLM execution |
| `running` | LLM call in progress |
| `completed` | LLM returned successfully; output stored |
| `failed` | LLM call failed or timed out |

### `RunField.type` values

| Value | Description |
|---|---|
| `string` | Single-line text |
| `text` | Multi-line text |
| `number` | Numeric input |
| `boolean` | True / false |
| `select` | One of a fixed list; `options` array is populated |

---

## 3. Relationship Types

Edges are stored in `ai_relationships`. Each edge has `_from`, `_to`, `name` (the label),
and an optional `properties` map.

**`ToMany=false`** — at most one edge of that label from the source.
**`ToMany=true`** — collection of edges.
**`Inverse`** — `CreateRelationship` writes both forward + inverse edges atomically.

| Label | From | To | ToMany | Inverse |
|---|---|---|---|---|
| `has_run` | `Agent` | `AgentRun` | ✅ | `belongs_to_agent` |
| `has_field` | `AgentRun` | `RunField` | ✅ | `belongs_to_run` |
| `has_input` | `AgentRun` | `RunInput` | ✅ | `belongs_to_run` |

---

## 4. Pre-Delivered Schema (`schema.go`)

`DefaultAISchema()` returns the fixed `types.Schema` seeded by `cmd/main.go`
on startup (idempotent via `AISchemaManager.SetSchema`).

### TypeDefinition: Agent

```go
{
    TypeID:   "Agent",
    Mutable:  true,
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "name",          Type: "string",  Required: true},
        {Name: "description",   Type: "string"},
        {Name: "provider",      Type: "string",  Required: true},
        {Name: "model",         Type: "string",  Required: true},
        {Name: "system_prompt", Type: "string",  Required: true},
        {Name: "temperature",   Type: "number"},
        {Name: "max_tokens",    Type: "number"},
    },
    Relationships: []types.RelationshipDefinition{
        {Label: "has_run", TargetTypeID: "AgentRun", ToMany: true,
         Inverse: "belongs_to_agent"},
    },
}
```

### TypeDefinition: AgentRun

```go
{
    TypeID:   "AgentRun",
    Mutable:  true,
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "agent_id",      Type: "string",  Required: true},
        {Name: "workflow_id",   Type: "string"},
        {Name: "instructions",  Type: "string",  Required: true},
        {Name: "status",        Type: "string",  Required: true},
        {Name: "output",        Type: "string"},
        {Name: "error_message", Type: "string"},
        {Name: "input_tokens",  Type: "number"},
        {Name: "output_tokens", Type: "number"},
        {Name: "started_at",    Type: "string"},
        {Name: "completed_at",  Type: "string"},
    },
    Relationships: []types.RelationshipDefinition{
        {Label: "has_field", TargetTypeID: "RunField", ToMany: true,
         Inverse: "belongs_to_run"},
        {Label: "has_input", TargetTypeID: "RunInput", ToMany: true,
         Inverse: "belongs_to_run"},
    },
}
```

### TypeDefinition: RunField

```go
{
    TypeID:   "RunField",
    Mutable:  false,   // Immutable — written once at Intake
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "fieldname",  Type: "string",  Required: true},
        {Name: "type",       Type: "string",  Required: true},
        {Name: "label",      Type: "string",  Required: true},
        {Name: "required",   Type: "boolean", Required: true},
        {Name: "options",    Type: "string"},  // JSON-encoded []string for type="select"
        {Name: "ordinality", Type: "number",  Required: true},
    },
}
```

### TypeDefinition: RunInput

```go
{
    TypeID:   "RunInput",
    Mutable:  false,   // Immutable — written once at Execute
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "fieldname", Type: "string", Required: true},
        {Name: "value",     Type: "string", Required: true},
    },
}
```
```
