// Package codevaldai — exported sentinel errors.
// All domain errors are defined here so that callers can type-switch
// without importing internal packages.
package codevaldai

import "errors"

var (
	// ErrProviderNotFound is returned when a requested LLMProvider ID does not exist.
	ErrProviderNotFound = errors.New("llm provider not found")

	// ErrProviderInUse is returned when DeleteProvider is called on a provider
	// that has one or more Agents referencing it via a uses_provider edge.
	ErrProviderInUse = errors.New("llm provider is in use by one or more agents")

	// ErrInvalidProvider is returned when CreateProvider is called with missing
	// required fields or an unsupported provider_type value.
	ErrInvalidProvider = errors.New("invalid provider: missing required fields or unsupported type")

	// ErrAgentNotFound is returned when an Agent lookup finds no matching record.
	ErrAgentNotFound = errors.New("agent not found")

	// ErrRunNotFound is returned when an AgentRun lookup finds no matching record.
	ErrRunNotFound = errors.New("agent run not found")

	// ErrRunNotIntaked is returned by ExecuteRun when the target AgentRun is
	// not in the pending_intake state.
	ErrRunNotIntaked = errors.New("run is not in pending_intake state")

	// ErrInvalidRunStatus is returned when a requested status transition is
	// not permitted by the run lifecycle.
	ErrInvalidRunStatus = errors.New("invalid run status transition")

	// ErrInvalidAgent is returned when a CreateAgent request is missing one
	// or more required fields (Name, ProviderID, Model, or SystemPrompt).
	ErrInvalidAgent = errors.New("invalid agent: missing required fields")

	// ErrAgentHasActiveRuns is returned by DeleteAgent when the agent still
	// has runs that are not in a terminal state (completed or failed).
	ErrAgentHasActiveRuns = errors.New("agent has active runs")

	// ErrInvalidLLMResponse is returned when the LLM response cannot be
	// parsed into the expected structure (e.g. []RunField at Intake).
	ErrInvalidLLMResponse = errors.New("invalid LLM response format")
)
