package codevaldai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Action is a single PubSub event the LLM wants CodeValdAI to publish
// on its behalf. The LLM embeds a fenced ```actions block in its output.
//
// Format the LLM must produce:
//
//	```actions
//	[{"topic":"git.branch.create","payload":{"repository":"gittesting","name":"feature/UTIL-001-foo"}}]
//	```
type Action struct {
	Topic   string         `json:"topic"`
	Payload map[string]any `json:"payload"`
}

// RawPayload returns the action payload serialised as a JSON string.
// Returns "{}" when the payload is nil or empty.
func (a Action) RawPayload() string {
	if len(a.Payload) == 0 {
		return "{}"
	}
	b, err := json.Marshal(a.Payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// parseActions extracts the ```actions block from the LLM output.
// Returns (nil, nil) when no block is present — callers treat that as a no-op.
// Returns a non-nil error when a fence is found but the block is malformed,
// so callers can log the format violation rather than silently dropping it.
func parseActions(output string) ([]Action, error) {
	const fence = "```actions"
	start := strings.Index(output, fence)
	if start == -1 {
		return nil, nil
	}
	rest := output[start+len(fence):]
	end := strings.Index(rest, "```")
	if end == -1 {
		return nil, fmt.Errorf("actions block has opening fence but no closing ```")
	}
	raw := strings.TrimSpace(rest[:end])
	if raw == "" {
		return nil, fmt.Errorf("actions block is empty")
	}
	var actions []Action
	if err := json.Unmarshal([]byte(raw), &actions); err != nil {
		return nil, fmt.Errorf("actions block contains invalid JSON: %w", err)
	}
	return actions, nil
}

// CatalogueEntry describes one PubSub topic a service is known to consume,
// expressed as an action the LLM may trigger.
type CatalogueEntry struct {
	ServiceName string
	Topic       string
	Direction   string // "consumes" | "produces"
}

// FormatActionCatalogue renders the catalogue as a human-readable block
// suitable for injection into the LLM system prompt.
func FormatActionCatalogue(entries []CatalogueEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Available Actions\n")
	b.WriteString("Publish one or more of these PubSub topics by appending an ```actions block to your response.\n\n")
	b.WriteString("| topic | handled by |\n")
	b.WriteString("|-------|------------|\n")
	for _, e := range entries {
		if e.Direction == "consumes" {
			b.WriteString("| ")
			b.WriteString(e.Topic)
			b.WriteString(" | ")
			b.WriteString(e.ServiceName)
			b.WriteString(" |\n")
		}
	}
	b.WriteString("\nOutput format (append at the end of your response):\n")
	b.WriteString("```actions\n")
	b.WriteString(`[{"topic":"<topic>","payload":{...}}]`)
	b.WriteString("\n```\n")
	return b.String()
}
