package codevaldai_test

import (
	"testing"

	codevaldai "github.com/aosanya/CodeValdAI"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// TestDefaultAISchema_NonZero verifies that DefaultAISchema returns a
// non-zero types.Schema with a populated ID and at least one TypeDefinition.
func TestDefaultAISchema_NonZero(t *testing.T) {
	s := codevaldai.DefaultAISchema()
	if s.ID == "" {
		t.Error("expected Schema.ID to be non-empty")
	}
	if len(s.Types) == 0 {
		t.Error("expected Schema.Types to be non-empty")
	}
}

// TestDefaultAISchema_AllTypeIDs verifies that all four expected TypeDefinitions
// are present in the schema.
func TestDefaultAISchema_AllTypeIDs(t *testing.T) {
	s := codevaldai.DefaultAISchema()

	want := []string{"Agent", "AgentRun", "RunField", "RunInput"}
	found := make(map[string]bool)
	for _, td := range s.Types {
		found[td.Name] = true
	}

	for _, name := range want {
		if !found[name] {
			t.Errorf("expected TypeDefinition %q to be present in schema", name)
		}
	}
}

// TestDefaultAISchema_AgentRequiredFields verifies that the Agent TypeDefinition
// has the expected required properties.
func TestDefaultAISchema_AgentRequiredFields(t *testing.T) {
	td := findType(t, codevaldai.DefaultAISchema(), "Agent")

	required := []string{"name", "provider", "model", "system_prompt"}
	assertRequiredProps(t, td, required)
}

// TestDefaultAISchema_AgentRunRequiredFields verifies that the AgentRun TypeDefinition
// has the expected required properties.
func TestDefaultAISchema_AgentRunRequiredFields(t *testing.T) {
	td := findType(t, codevaldai.DefaultAISchema(), "AgentRun")

	required := []string{"agent_id", "instructions", "status"}
	assertRequiredProps(t, td, required)
}

// TestDefaultAISchema_RunFieldRequiredFields verifies that the RunField TypeDefinition
// has the expected required properties and is marked Immutable.
func TestDefaultAISchema_RunFieldRequiredFields(t *testing.T) {
	td := findType(t, codevaldai.DefaultAISchema(), "RunField")

	if !td.Immutable {
		t.Error("expected RunField.Immutable to be true")
	}

	required := []string{"fieldname", "type", "label", "required", "ordinality"}
	assertRequiredProps(t, td, required)
}

// TestDefaultAISchema_RunInputRequiredFields verifies that the RunInput TypeDefinition
// has the expected required properties and is marked Immutable.
func TestDefaultAISchema_RunInputRequiredFields(t *testing.T) {
	td := findType(t, codevaldai.DefaultAISchema(), "RunInput")

	if !td.Immutable {
		t.Error("expected RunInput.Immutable to be true")
	}

	required := []string{"fieldname", "value"}
	assertRequiredProps(t, td, required)
}

// TestDefaultAISchema_AgentRelationships verifies that Agent declares has_run
// pointing to AgentRun with inverse belongs_to_agent.
func TestDefaultAISchema_AgentRelationships(t *testing.T) {
	td := findType(t, codevaldai.DefaultAISchema(), "Agent")

	rel := findRelationship(t, td, "has_run")
	if rel.ToType != "AgentRun" {
		t.Errorf("has_run.ToType: got %q, want %q", rel.ToType, "AgentRun")
	}
	if !rel.ToMany {
		t.Error("has_run.ToMany should be true")
	}
	if rel.Inverse != "belongs_to_agent" {
		t.Errorf("has_run.Inverse: got %q, want %q", rel.Inverse, "belongs_to_agent")
	}
}

// TestDefaultAISchema_AgentRunRelationships verifies that AgentRun declares
// has_field → RunField and has_input → RunInput with the correct inverses.
func TestDefaultAISchema_AgentRunRelationships(t *testing.T) {
	td := findType(t, codevaldai.DefaultAISchema(), "AgentRun")

	hasField := findRelationship(t, td, "has_field")
	if hasField.ToType != "RunField" {
		t.Errorf("has_field.ToType: got %q, want %q", hasField.ToType, "RunField")
	}
	if hasField.Inverse != "belongs_to_run" {
		t.Errorf("has_field.Inverse: got %q, want %q", hasField.Inverse, "belongs_to_run")
	}

	hasInput := findRelationship(t, td, "has_input")
	if hasInput.ToType != "RunInput" {
		t.Errorf("has_input.ToType: got %q, want %q", hasInput.ToType, "RunInput")
	}
	if hasInput.Inverse != "belongs_to_run" {
		t.Errorf("has_input.Inverse: got %q, want %q", hasInput.Inverse, "belongs_to_run")
	}
}

// TestDefaultAISchema_StorageCollection verifies that all types route to
// ai_entities.
func TestDefaultAISchema_StorageCollection(t *testing.T) {
	s := codevaldai.DefaultAISchema()
	for _, td := range s.Types {
		if td.StorageCollection != "ai_entities" {
			t.Errorf("TypeDefinition %q: StorageCollection = %q, want %q",
				td.Name, td.StorageCollection, "ai_entities")
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func findType(t *testing.T, s types.Schema, name string) types.TypeDefinition {
	t.Helper()
	for _, td := range s.Types {
		if td.Name == name {
			return td
		}
	}
	t.Fatalf("TypeDefinition %q not found in schema", name)
	return types.TypeDefinition{}
}

func findRelationship(t *testing.T, td types.TypeDefinition, name string) types.RelationshipDefinition {
	t.Helper()
	for _, rel := range td.Relationships {
		if rel.Name == name {
			return rel
		}
	}
	t.Fatalf("RelationshipDefinition %q not found on TypeDefinition %q", name, td.Name)
	return types.RelationshipDefinition{}
}

func assertRequiredProps(t *testing.T, td types.TypeDefinition, names []string) {
	t.Helper()
	propMap := make(map[string]types.PropertyDefinition)
	for _, p := range td.Properties {
		propMap[p.Name] = p
	}
	for _, name := range names {
		p, ok := propMap[name]
		if !ok {
			t.Errorf("TypeDefinition %q: property %q not found", td.Name, name)
			continue
		}
		if !p.Required {
			t.Errorf("TypeDefinition %q: property %q should be Required=true", td.Name, name)
		}
	}
}
