# CodeValdAI — Graph Topology & Schema

> Part of the split architecture. Index: [architecture.md](architecture.md)

---

## 1. Graph Topology

```
LLMProvider ◄──uses_provider── Agent ──has_run──► AgentRun ──has_field──► RunField
                                                             ──has_input──► RunInput
```

All nodes and edges live in two ArangoDB collections:
- **`ai_entities`** — document collection (LLMProvider, Agent, AgentRun, RunField, RunInput)
- **`ai_relationships`** — **edge** collection (all relationship types)

---

## 2. Entity Types

| Type | Mutable | Properties | Notes |
|---|---|---|---|
| `LLMProvider` | ✅ | `name`(req), `provider_type`(req), `api_key`(req), `base_url` | Reusable LLM config; shared across Agents. `base_url` empty = use provider default |
| `Agent` | ✅ | `name`(req), `description`, `model`(req), `system_prompt`(req), `temperature`, `max_tokens` | References one `LLMProvider` via `uses_provider` edge |
| `AgentRun` | ✅ | `instructions`(req), `status`(req), `output`, `error_message`, `input_tokens`, `output_tokens`, `started_at`, `completed_at` | Two-phase execution record; linked to Agent via `belongs_to_agent` edge |
| `RunField` | ❌ immutable | `fieldname`(req), `type`(req), `label`(req), `required`(req), `options`, `ordinality`(req) | Inferred by LLM during Intake; written once |
| `RunInput` | ❌ immutable | `fieldname`(req), `value`(req) | Submitted by caller during Execute; written once |

### `provider_type` values

| Value | Description |
|---|---|
| `anthropic` | Anthropic Messages API (`POST /v1/messages`) |
| `openai` | Reserved; not implemented in MVP |

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
**`Required=true`** — must be supplied when creating the entity.

### Forward relationships

| Label | From | To | ToMany | Inverse |
|---|---|---|---|---|
| `uses_provider` | `Agent` | `LLMProvider` | ❌ | `used_by_agent` |
| `has_run` | `Agent` | `AgentRun` | ✅ | `belongs_to_agent` |
| `has_field` | `AgentRun` | `RunField` | ✅ | `belongs_to_run` |
| `has_input` | `AgentRun` | `RunInput` | ✅ | `belongs_to_run` |

### Inverse relationships (auto-written by `CreateRelationship`)

| Label | On Type | Points To | Required |
|---|---|---|---|
| `used_by_agent` | `LLMProvider` | `Agent` | — |
| `belongs_to_agent` | `AgentRun` | `Agent` | ✅ |
| `belongs_to_run` | `RunField` | `AgentRun` | ✅ |
| `belongs_to_run` | `RunInput` | `AgentRun` | ✅ |

---

## 4. Pre-Delivered Schema (`schema.go`)

`DefaultAISchema()` returns the fixed `types.Schema` seeded by `cmd/main.go`
on startup (idempotent via `AISchemaManager.SetSchema`).

### TypeDefinition: LLMProvider

```go
{
    Name:              "LLMProvider",
    DisplayName:       "LLM Provider",
    PathSegment:       "providers",
    EntityIDParam:     "providerId",
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "name",          Type: types.PropertyTypeString, Required: true},
        {Name: "provider_type", Type: types.PropertyTypeString, Required: true}, // "anthropic" | "openai"
        {Name: "api_key",       Type: types.PropertyTypeString, Required: true},
        {Name: "base_url",      Type: types.PropertyTypeString},                 // empty = use provider default
        {Name: "created_at",    Type: types.PropertyTypeString},
        {Name: "updated_at",    Type: types.PropertyTypeString},
    },
    Relationships: []types.RelationshipDefinition{
        {Name: "used_by_agent", Label: "Agents", ToType: "Agent", ToMany: true, Inverse: "uses_provider"},
    },
}
```

### TypeDefinition: Agent

```go
{
    Name:              "Agent",
    DisplayName:       "Agent",
    PathSegment:       "agents",
    EntityIDParam:     "agentId",
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "name",          Type: types.PropertyTypeString,  Required: true},
        {Name: "description",   Type: types.PropertyTypeString},
        {Name: "model",         Type: types.PropertyTypeString,  Required: true},
        {Name: "system_prompt", Type: types.PropertyTypeString,  Required: true},
        {Name: "temperature",   Type: types.PropertyTypeFloat},
        {Name: "max_tokens",    Type: types.PropertyTypeInteger},
        {Name: "created_at",    Type: types.PropertyTypeString},
        {Name: "updated_at",    Type: types.PropertyTypeString},
    },
    Relationships: []types.RelationshipDefinition{
        {Name: "uses_provider", Label: "Provider", PathSegment: "provider",
         ToType: "LLMProvider", ToMany: false, Required: true, Inverse: "used_by_agent"},
        {Name: "has_run", Label: "Runs", PathSegment: "runs",
         ToType: "AgentRun", ToMany: true, Inverse: "belongs_to_agent"},
    },
}
```

### TypeDefinition: AgentRun

```go
{
    Name:              "AgentRun",
    DisplayName:       "Agent Run",
    PathSegment:       "runs",
    EntityIDParam:     "runId",
    StorageCollection: "ai_entities",
    Properties: []types.PropertyDefinition{
        {Name: "instructions",  Type: types.PropertyTypeString,  Required: true},
        {Name: "status",        Type: types.PropertyTypeString,  Required: true},
        {Name: "output",        Type: types.PropertyTypeString},
        {Name: "error_message", Type: types.PropertyTypeString},
        {Name: "input_tokens",  Type: types.PropertyTypeInteger},
        {Name: "output_tokens", Type: types.PropertyTypeInteger},
        {Name: "started_at",    Type: types.PropertyTypeString},
        {Name: "completed_at",  Type: types.PropertyTypeString},
        {Name: "created_at",    Type: types.PropertyTypeString},
        {Name: "updated_at",    Type: types.PropertyTypeString},
    },
    Relationships: []types.RelationshipDefinition{
        {Name: "belongs_to_agent", Label: "Agent", PathSegment: "agent",
         ToType: "Agent", ToMany: false, Required: true, Inverse: "has_run"},
        {Name: "has_field", Label: "Fields", PathSegment: "fields",
         ToType: "RunField", ToMany: true, Inverse: "belongs_to_run"},
        {Name: "has_input", Label: "Inputs", PathSegment: "inputs",
         ToType: "RunInput", ToMany: true, Inverse: "belongs_to_run"},
    },
}
```

### TypeDefinition: RunField

```go
{
    Name:              "RunField",
    DisplayName:       "Run Field",
    StorageCollection: "ai_entities",
    Immutable:         true,
    Properties: []types.PropertyDefinition{
        {Name: "fieldname",  Type: types.PropertyTypeString,  Required: true},
        {Name: "type",       Type: types.PropertyTypeString,  Required: true},
        {Name: "label",      Type: types.PropertyTypeString,  Required: true},
        {Name: "required",   Type: types.PropertyTypeBoolean, Required: true},
        {Name: "options",    Type: types.PropertyTypeString},  // JSON-encoded []string for type="select"
        {Name: "ordinality", Type: types.PropertyTypeInteger, Required: true},
    },
    Relationships: []types.RelationshipDefinition{
        {Name: "belongs_to_run", Label: "Run", ToType: "AgentRun",
         ToMany: false, Required: true, Inverse: "has_field"},
    },
}
```

### TypeDefinition: RunInput

```go
{
    Name:              "RunInput",
    DisplayName:       "Run Input",
    StorageCollection: "ai_entities",
    Immutable:         true,
    Properties: []types.PropertyDefinition{
        {Name: "fieldname", Type: types.PropertyTypeString, Required: true},
        {Name: "value",     Type: types.PropertyTypeString, Required: true},
    },
    Relationships: []types.RelationshipDefinition{
        {Name: "belongs_to_run", Label: "Run", ToType: "AgentRun",
         ToMany: false, Required: true, Inverse: "has_input"},
    },
}
```
