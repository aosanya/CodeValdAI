// Package codevaldai — pre-delivered schema definition.
//
// This file exposes [DefaultAISchema], which returns the fixed [types.Schema]
// for CodeValdAI. internal/app seeds this schema idempotently on startup via
// [entitygraph.SeedSchema].
//
// The schema declares five TypeDefinitions:
//   - LLMProvider — reusable LLM configuration entity (mutable)
//   - Agent       — AI agent configuration entity (mutable)
//   - AgentRun    — two-phase execution record (mutable)
//   - RunField    — input field inferred by the LLM during Intake (immutable)
//   - RunInput    — filled value submitted by the caller during Execute (immutable)
//
// Graph topology:
//
// LLMProvider ◄──uses_provider── Agent ──has_run──► AgentRun ──has_field──► RunField
//
//	──has_input──► RunInput
//
// Storage: all five types are stored in the "ai_entities" document collection.
// Edges live in the "ai_relationships" edge collection.
//
// Inverse relationships auto-created by [entitygraph.DataManager.CreateRelationship]:
//
// Agent     ──used_by_agent──►    LLMProvider
// AgentRun  ──belongs_to_agent──► Agent
// RunField  ──belongs_to_run──►   AgentRun
// RunInput  ──belongs_to_run──►   AgentRun
package codevaldai

import (
	"github.com/aosanya/CodeValdSharedLib/eventreceiver"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// DefaultAISchema returns the pre-delivered [types.Schema] seeded by
// internal/app on startup via [entitygraph.SeedSchema]. The operation is
// idempotent — calling it multiple times with the same schema ID is safe.
//
// The returned schema contains TypeDefinitions for LLMProvider, Agent,
// AgentRun, RunField, RunInput, and ReceivedEvent, matching the specification
// in documentation/2-SoftwareDesignAndArchitecture/architecture-graph.md §4.
func DefaultAISchema() types.Schema {
	return types.Schema{
		ID:      "ai-schema-v1",
		Version: 1,
		Tag:     "v1",
		Types: append(aiTypes(), eventreceiver.ReceivedEventTypeDefinition("ai")),
	}
}

func aiTypes() []types.TypeDefinition {
	return []types.TypeDefinition{
			{
				Name:              "LLMProvider",
				DisplayName:       "LLM Provider",
				PathSegment:       "providers",
				EntityIDParam:     "providerId",
				StorageCollection: "ai_entities",
				Properties: []types.PropertyDefinition{
					{Name: "ref_code", Type: types.PropertyTypeUUID, Required: true},
					{Name: "code", Type: types.PropertyTypeString, Required: false},
					{Name: "name", Type: types.PropertyTypeString, Required: true},
					// provider_type: "anthropic" | "openai" | "huggingface"
					{Name: "provider_type", Type: types.PropertyTypeString, Required: true},
					{Name: "api_key", Type: types.PropertyTypeString, Required: true},
					// base_url: empty string means use the provider's default endpoint.
					{Name: "base_url", Type: types.PropertyTypeString},
					// provider_route: HuggingFace-only backend pin (e.g. "fireworks-ai");
					// ignored for "anthropic" and "openai".
					{Name: "provider_route", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:    "used_by_agent",
						Label:   "Agents",
						ToType:  "Agent",
						ToMany:  true,
						Inverse: "uses_provider",
					},
				},
			},
			{
				Name:              "Agent",
				DisplayName:       "Agent",
				PathSegment:       "agents",
				EntityIDParam:     "agentId",
				StorageCollection: "ai_entities",
				Properties: []types.PropertyDefinition{
					{Name: "ref_code", Type: types.PropertyTypeUUID, Required: true},
					{Name: "code", Type: types.PropertyTypeString, Required: false},
					{Name: "name", Type: types.PropertyTypeString, Required: true},
					{Name: "description", Type: types.PropertyTypeString},
					{Name: "model", Type: types.PropertyTypeString, Required: true},
					{Name: "system_prompt", Type: types.PropertyTypeString, Required: true},
					{Name: "temperature", Type: types.PropertyTypeFloat},
					{Name: "max_tokens", Type: types.PropertyTypeInteger},
					// timeout_seconds: per-Agent override of the system default
					// LLM-call timeout. Zero means use the default.
					{Name: "timeout_seconds", Type: types.PropertyTypeInteger},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        "uses_provider",
						Label:       "Provider",
						PathSegment: "provider",
						ToType:      "LLMProvider",
						ToMany:      false,
						Required:    true,
						Inverse:     "used_by_agent",
					},
					{
						Name:        "has_run",
						Label:       "Runs",
						PathSegment: "runs",
						ToType:      "AgentRun",
						ToMany:      true,
						Inverse:     "belongs_to_agent",
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
					{Name: "ref_code", Type: types.PropertyTypeUUID, Required: true},
					{Name: "code", Type: types.PropertyTypeString, Required: false},
					{Name: "instructions", Type: types.PropertyTypeString, Required: true},
					{Name: "status", Type: types.PropertyTypeString, Required: true},
					{Name: "output", Type: types.PropertyTypeString},
					{Name: "error_message", Type: types.PropertyTypeString},
					{Name: "input_tokens", Type: types.PropertyTypeInteger},
					{Name: "output_tokens", Type: types.PropertyTypeInteger},
					{Name: "started_at", Type: types.PropertyTypeString},
					{Name: "completed_at", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        "belongs_to_agent",
						Label:       "Agent",
						PathSegment: "agent",
						ToType:      "Agent",
						ToMany:      false,
						Required:    true,
						Inverse:     "has_run",
					},
					{
						Name:        "has_field",
						Label:       "Fields",
						PathSegment: "fields",
						ToType:      "RunField",
						ToMany:      true,
						Inverse:     "belongs_to_run",
					},
					{
						Name:        "has_input",
						Label:       "Inputs",
						PathSegment: "inputs",
						ToType:      "RunInput",
						ToMany:      true,
						Inverse:     "belongs_to_run",
					},
				},
			},
			{
				Name:              "RunField",
				DisplayName:       "Run Field",
				StorageCollection: "ai_entities",
				Immutable:         true,
				Properties: []types.PropertyDefinition{
					{Name: "ref_code", Type: types.PropertyTypeUUID, Required: true},
					{Name: "code", Type: types.PropertyTypeString, Required: false},
					{Name: "fieldname", Type: types.PropertyTypeString, Required: true},
					{Name: "type", Type: types.PropertyTypeString, Required: true},
					{Name: "label", Type: types.PropertyTypeString, Required: true},
					{Name: "required", Type: types.PropertyTypeBoolean, Required: true},
					// options is a JSON-encoded []string; populated only when type="select".
					{Name: "options", Type: types.PropertyTypeString},
					{Name: "ordinality", Type: types.PropertyTypeInteger, Required: true},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:     "belongs_to_run",
						Label:    "Run",
						ToType:   "AgentRun",
						ToMany:   false,
						Required: true,
						Inverse:  "has_field",
					},
				},
			},
			{
				Name:              "RunInput",
				DisplayName:       "Run Input",
				StorageCollection: "ai_entities",
				Immutable:         true,
				Properties: []types.PropertyDefinition{
					{Name: "ref_code", Type: types.PropertyTypeUUID, Required: true},
					{Name: "code", Type: types.PropertyTypeString, Required: false},
					{Name: "fieldname", Type: types.PropertyTypeString, Required: true},
					{Name: "value", Type: types.PropertyTypeString, Required: true},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:     "belongs_to_run",
						Label:    "Run",
						ToType:   "AgentRun",
						ToMany:   false,
						Required: true,
						Inverse:  "has_input",
					},
				},
			},
	}
}
