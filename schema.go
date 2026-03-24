// Package codevaldai — pre-delivered schema definition.
//
// This file exposes [DefaultAISchema], which returns the fixed [types.Schema]
// for CodeValdAI. cmd/main.go seeds this schema idempotently on startup via
// AISchemaManager.SetSchema.
//
// The schema declares four TypeDefinitions:
//   - Agent       — root LLM configuration entity (mutable)
//   - AgentRun    — two-phase execution record (mutable)
//   - RunField    — input field inferred by the LLM during Intake (immutable)
//   - RunInput    — filled value submitted by the caller during Execute (immutable)
//
// Graph topology:
//
//	Agent ──has_run──► AgentRun ──has_field──► RunField
//	                             ──has_input──► RunInput
//
// Storage: all four types are stored in the "ai_entities" document collection.
// Edges live in the "ai_relationships" edge collection.
//
// Inverse relationships auto-created by [entitygraph.DataManager.CreateRelationship]:
//
//	AgentRun  ──belongs_to_agent──► Agent
//	RunField  ──belongs_to_run──►  AgentRun
//	RunInput  ──belongs_to_run──►  AgentRun
package codevaldai

import "github.com/aosanya/CodeValdSharedLib/types"

// DefaultAISchema returns the pre-delivered [types.Schema] seeded by
// cmd/main.go on startup via AISchemaManager.SetSchema. The operation is
// idempotent — calling it multiple times with the same schema ID is safe.
//
// The returned schema contains TypeDefinitions for Agent, AgentRun, RunField,
// and RunInput, matching the specification in
// documentation/2-SoftwareDesignAndArchitecture/architecture-graph.md §4.
func DefaultAISchema() types.Schema {
	return types.Schema{
		ID:      "ai-schema-v1",
		Version: 1,
		Tag:     "v1",
		Types: []types.TypeDefinition{
			{
				Name:              "Agent",
				DisplayName:       "Agent",
				PathSegment:       "agents",
				EntityIDParam:     "agentId",
				StorageCollection: "ai_entities",
				Properties: []types.PropertyDefinition{
					{Name: "name",          Type: types.PropertyTypeString,  Required: true},
					{Name: "description",   Type: types.PropertyTypeString},
					{Name: "provider",      Type: types.PropertyTypeString,  Required: true},
					{Name: "model",         Type: types.PropertyTypeString,  Required: true},
					{Name: "system_prompt", Type: types.PropertyTypeString,  Required: true},
					{Name: "temperature",   Type: types.PropertyTypeFloat},
					{Name: "max_tokens",    Type: types.PropertyTypeInteger},
					{Name: "created_at",    Type: types.PropertyTypeString},
					{Name: "updated_at",    Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:    "has_run",
						Label:   "Runs",
						ToType:  "AgentRun",
						ToMany:  true,
						Inverse: "belongs_to_agent",
					},
				},
			},
			{
				Name:              "AgentRun",
				DisplayName:       "Agent Run",
				PathSegment:       "runs",
				EntityIDParam:     "runId",
				StorageCollection: "ai_entities",
				Properties: []types.PropertyDefinition{
					{Name: "agent_id",      Type: types.PropertyTypeString,  Required: true},
					{Name: "workflow_id",   Type: types.PropertyTypeString},
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
					{
						Name:    "belongs_to_agent",
						Label:   "Agent",
						ToType:  "Agent",
						ToMany:  false,
						Required: true,
						Inverse: "has_run",
					},
					{
						Name:    "has_field",
						Label:   "Fields",
						ToType:  "RunField",
						ToMany:  true,
						Inverse: "belongs_to_run",
					},
					{
						Name:    "has_input",
						Label:   "Inputs",
						ToType:  "RunInput",
						ToMany:  true,
						Inverse: "belongs_to_run",
					},
				},
			},
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
					// options is a JSON-encoded []string; populated only when type="select".
					{Name: "options",    Type: types.PropertyTypeString},
					{Name: "ordinality", Type: types.PropertyTypeInteger, Required: true},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:    "belongs_to_run",
						Label:   "Run",
						ToType:  "AgentRun",
						ToMany:  false,
						Required: true,
						Inverse: "has_field",
					},
				},
			},
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
					{
						Name:    "belongs_to_run",
						Label:   "Run",
						ToType:  "AgentRun",
						ToMany:  false,
						Required: true,
						Inverse: "has_input",
					},
				},
			},
		},
	}
}
