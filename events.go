package codevaldai

// AI event topics — CodeValdAI publishes only ai.* events.
// No agencyID segment: each service instance is scoped to a single agency.
const (
	// Run lifecycle (task-driven)
	TopicTaskInProgress = "ai.task.in_progress"
	TopicTaskCompleted  = "ai.task.completed"
	TopicTaskFailed     = "ai.task.failed"

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
