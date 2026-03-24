```markdown
# CodeValdAI — ArangoDB Storage

> Part of the split architecture. Index: [architecture.md](architecture.md)

---

## 1. Collections

| Collection | Type | Contents |
|---|---|---|
| `ai_entities` | Document | Agent, AgentRun, RunField, RunInput |
| `ai_relationships` | **Edge** | All relationship edges (`has_run`, `has_field`, `has_input` + inverses) |
| `ai_schemas` | Document | Pre-delivered schema version documents (managed by `AISchemaManager`) |

`ai_relationships` **must** be created as an edge collection — not a regular
document collection. ArangoDB graph traversal requires edge collections.

---

## 2. Document Shapes

### `ai_entities/{key}` — Agent

```json
{
  "_key":      "<uuid>",
  "type_id":   "Agent",
  "agency_id": "<agencyID>",
  "properties": {
    "name":          "Risk Analyst",
    "description":   "Identifies and summarises operational risk",
    "provider":      "anthropic",
    "model":         "claude-3-5-sonnet-20241022",
    "system_prompt": "You are a senior risk analyst...",
    "temperature":   0.7,
    "max_tokens":    4096
  },
  "created_at": "2026-03-24T10:00:00Z",
  "updated_at": "2026-03-24T10:00:00Z"
}
```

### `ai_entities/{key}` — AgentRun

```json
{
  "_key":      "<uuid>",
  "type_id":   "AgentRun",
  "agency_id": "<agencyID>",
  "properties": {
    "agent_id":      "<agentID>",
    "workflow_id":   "<workflowID>",
    "instructions":  "Analyse the attached report and summarise key risks.",
    "status":        "completed",
    "output":        "The Q1 2026 Risk Report identifies three primary...",
    "error_message": "",
    "input_tokens":  1240,
    "output_tokens": 876,
    "started_at":    "2026-03-24T10:04:00Z",
    "completed_at":  "2026-03-24T10:05:00Z"
  },
  "created_at": "2026-03-24T10:03:00Z",
  "updated_at": "2026-03-24T10:05:00Z"
}
```

### `ai_entities/{key}` — RunField

```json
{
  "_key":      "<uuid>",
  "type_id":   "RunField",
  "agency_id": "<agencyID>",
  "properties": {
    "fieldname":  "risk_category",
    "type":       "select",
    "label":      "Risk Category",
    "required":   true,
    "options":    "[\"financial\",\"operational\",\"reputational\"]",
    "ordinality": 2
  },
  "created_at": "2026-03-24T10:03:00Z",
  "updated_at": "2026-03-24T10:03:00Z"
}
```

### `ai_entities/{key}` — RunInput

```json
{
  "_key":      "<uuid>",
  "type_id":   "RunInput",
  "agency_id": "<agencyID>",
  "properties": {
    "fieldname": "risk_category",
    "value":     "financial"
  },
  "created_at": "2026-03-24T10:04:00Z",
  "updated_at": "2026-03-24T10:04:00Z"
}
```

### `ai_relationships/{key}` — Edge

```json
{
  "_key":  "<uuid>",
  "_from": "ai_entities/<agentID>",
  "_to":   "ai_entities/<runID>",
  "name":  "has_run",
  "properties": {}
}
```

---

## 3. Indexes

| Collection | Index | Fields | Type | Notes |
|---|---|---|---|---|
| `ai_entities` | `idx_type_agency` | `type_id`, `agency_id` | persistent | List by type within an agency |
| `ai_entities` | `idx_run_status` | `properties.status` | persistent | Filter runs by status |
| `ai_entities` | `idx_run_agent` | `properties.agent_id` | persistent | List runs by agent |
| `ai_relationships` | `idx_from_name` | `_from`, `name` | persistent | Traverse outbound edges by label |
| `ai_relationships` | `idx_to_name` | `_to`, `name` | persistent | Traverse inbound edges by label |

---

## 4. Named Graph

Graph name: `ai_graph`

| Component | Collection |
|---|---|
| Edge collection | `ai_relationships` |
| Vertex collections | `ai_entities` |

Used by `TraverseGraph` in `entitygraph.DataManager` for depth-first traversal.
```
