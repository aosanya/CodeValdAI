package codevaldai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// intakeSystemMessage is the fixed system prompt sent to the LLM during the
// Intake phase. It instructs the model to return only a JSON array of input
// field descriptors — no prose, no markdown.
const intakeSystemMessage = `You are an AI intake assistant. Given a task instruction, return ONLY a JSON array of input fields required to complete the task. Each object in the array must have:
  fieldname (string, snake_case)
  type      (one of: string | text | number | boolean | select)
  label     (human-readable label)
  required  (bool)
  options   ([]string, only present when type=select)
Return nothing else — no prose, no markdown, only the JSON array.`

// intakeField is the internal struct used to unmarshal the LLM's JSON response.
type intakeField struct {
	Fieldname string   `json:"fieldname"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Required  bool     `json:"required"`
	Options   []string `json:"options,omitempty"`
}

// IntakeRun implements AIManager.IntakeRun.
// Phase 1 of the two-phase run lifecycle: calls the LLM to infer what input
// fields are needed, persists an AgentRun in pending_intake, and stores the
// inferred RunFields linked via has_field edges.
//
// Returns ErrInvalidAgent if agent_id or instructions are empty.
// Returns ErrAgentNotFound if no agent with that ID exists.
// Returns ErrProviderNotFound if the agent has no uses_provider edge.
// Returns ErrInvalidLLMResponse if the LLM returns unparseable or empty JSON.
func (m *aiManager) IntakeRun(ctx context.Context, req IntakeRunRequest) (AgentRun, []RunField, error) {
	if req.AgentID == "" || req.Instructions == "" {
		return AgentRun{}, nil, ErrInvalidAgent
	}

	agentEntity, err := m.dm.GetEntity(ctx, m.agencyID, req.AgentID)
	if err != nil {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: %w", req.AgentID, toAgentErr(err))
	}
	agent := agentFromEntity(agentEntity)

	rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: m.agencyID,
		FromID:   req.AgentID,
		Name:     "uses_provider",
	})
	if err != nil {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: lookup provider: %w", req.AgentID, err)
	}
	if len(rels) == 0 {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: %w", req.AgentID, ErrProviderNotFound)
	}

	providerEntity, err := m.dm.GetEntity(ctx, m.agencyID, rels[0].ToID)
	if err != nil {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: %w", req.AgentID, toProviderErr(err))
	}
	provider := providerFromEntity(providerEntity)

	userMsg := "Instructions: " + req.Instructions + "\nWhat input fields do you need to complete this task?"
	var buf strings.Builder
	if _, _, err := m.callLLM(ctx, provider, agent, intakeSystemMessage, userMsg, func(chunk string) { buf.WriteString(chunk) }); err != nil {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: llm: %w", req.AgentID, err)
	}

	fields, err := parseIntakeFields(buf.String())
	if err != nil || len(fields) == 0 {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: %w", req.AgentID, ErrInvalidLLMResponse)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	runEntity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: m.agencyID,
		TypeID:   "AgentRun",
		Properties: map[string]any{
			"instructions": req.Instructions,
			"status":       string(AgentRunStatusPendingIntake),
			"created_at":   now,
			"updated_at":   now,
		},
		Relationships: []entitygraph.EntityRelationshipRequest{
			{Name: "belongs_to_agent", ToID: req.AgentID},
		},
	})
	if err != nil {
		return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: create run: %w", req.AgentID, err)
	}

	runFields := make([]RunField, 0, len(fields))
	for i, f := range fields {
		fieldEntity, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
			AgencyID: m.agencyID,
			TypeID:   "RunField",
			Properties: map[string]any{
				"fieldname":  f.Fieldname,
				"type":       f.Type,
				"label":      f.Label,
				"required":   f.Required,
				"options":    marshalOptions(f.Options),
				"ordinality": i + 1,
			},
			Relationships: []entitygraph.EntityRelationshipRequest{
				{Name: "belongs_to_run", ToID: runEntity.ID},
			},
		})
		if err != nil {
			return AgentRun{}, nil, fmt.Errorf("IntakeRun %s: create field %d: %w", req.AgentID, i, err)
		}
		runFields = append(runFields, runFieldFromEntity(fieldEntity))
	}

	run := agentRunFromEntity(runEntity)
	run.AgentID = req.AgentID
	return run, runFields, nil
}

// parseIntakeFields unmarshals the LLM response into a slice of intakeField.
// If the response contains prose wrapping the JSON array, it extracts the
// [...] portion before parsing. Returns ErrInvalidLLMResponse on failure.
func parseIntakeFields(response string) ([]intakeField, error) {
	raw := extractJSONArray(response)
	if raw == "" {
		return nil, ErrInvalidLLMResponse
	}
	var fields []intakeField
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, ErrInvalidLLMResponse
	}
	return fields, nil
}

// extractJSONArray extracts the first [...] substring from s.
// Returns "" if no balanced array is found.
func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(s, "]")
	if end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}

// marshalOptions encodes a []string to a JSON string for ArangoDB storage.
// Returns "" for nil or empty slices.
func marshalOptions(opts []string) string {
	if len(opts) == 0 {
		return ""
	}
	b, _ := json.Marshal(opts)
	return string(b)
}

// unmarshalOptions decodes a JSON string back to []string.
// Returns nil for empty strings or invalid JSON.
func unmarshalOptions(s string) []string {
	if s == "" {
		return nil
	}
	var opts []string
	if err := json.Unmarshal([]byte(s), &opts); err != nil {
		return nil
	}
	return opts
}
