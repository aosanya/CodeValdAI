package codevaldai

// AI task lifecycle topics — CodeValdAI publishes only ai.* events.
// CodeValdWork consumes these and bridges them to work.task.* events.
const (
	TopicTaskInProgress = "ai.task.in_progress"
	TopicTaskCompleted  = "ai.task.completed"
	TopicTaskFailed     = "ai.task.failed"
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
