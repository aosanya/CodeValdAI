package codevaldai

// AI event topics — CodeValdAI publishes only ai.* events.
// No agencyID segment: each service instance is scoped to a single agency.
const (
	// Run lifecycle (task-driven)
	TopicTaskInProgress = "ai.task.in_progress"
	TopicTaskCompleted  = "ai.task.completed"
	TopicTaskFailed     = "ai.task.failed"
	// TopicTaskYielded is published when a session hits its wall-clock or token
	// limit and a successor session has been created to continue the chain.
	TopicTaskYielded = "ai.task.yielded"

	// Run lifecycle (internal / recovery)
	TopicRunCompleted = "ai.run.completed"
	TopicRunFailed    = "ai.run.failed"

	// Agent management
	TopicAgentCreated = "ai.agent.created"
)

// TaskInProgressPayload is published when ExecuteRunStreaming transitions to
// the running state (before the LLM call). Signals that work has begun.
type TaskInProgressPayload struct {
	TaskID  string
	RunID   string
	AgentID string
}

// TaskCompletedPayload is published when ExecuteRunStreaming finishes
// successfully and actions have been dispatched.
type TaskCompletedPayload struct {
	TaskID  string
	RunID   string
	AgentID string
}

// TaskFailedPayload is published when the LLM call errors, times out, or
// the output contains no actions block.
type TaskFailedPayload struct {
	TaskID  string
	RunID   string
	Reason  string
}

// TaskYieldedPayload is published when a session hits its wall-clock or token
// limit. A successor run has already been created at publish time.
type TaskYieldedPayload struct {
	TaskID        string
	RunID         string
	ChainID       string
	SegmentNumber int
	TokensUsed    int
	PartialOutput string
}
