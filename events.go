package codevaldai

// AI event topics — CodeValdAI publishes only ai.* events.
// No agencyID segment: each service instance is scoped to a single agency.
const (
	// Run lifecycle (task-driven)
	TopicTaskStarted   = "ai.task.started"
	TopicTaskCompleted = "ai.task.completed"
	TopicTaskFailed    = "ai.task.failed"
	// TopicTaskYielded is published when a session hits its wall-clock or token
	// limit and a successor session has been created to continue the chain.
	TopicTaskYielded = "ai.task.yielded"

	// Run lifecycle (internal / recovery)
	TopicRunCompleted = "ai.run.completed"
	TopicRunFailed    = "ai.run.failed"

	// Agent management
	TopicAgentCreated = "ai.agent.created"

	// Task decomposition
	// TopicTodoCreated is published when a developer agent decomposes an inbound
	// task into sub-tasks. CodeValdWork consumes this topic and materialises each
	// TodoItem as a TaskTodo entity, then publishes work.todo.dispatched so
	// CodeValdAI agents can pick each todo up via a work plan.
	TopicTodoCreated = "ai.todo.created"
)

// TaskStartedPayload is published when ExecuteRunStreaming transitions to
// the running state (before the LLM call). Signals that work has begun.
type TaskStartedPayload struct {
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

// TodoCreatedPayload is published on ai.todo.created when a developer agent
// decomposes an inbound task into sub-tasks.
type TodoCreatedPayload struct {
	ParentTaskID string     `json:"parent_task_id"` // Work task that triggered the decomposition
	RunID        string     `json:"run_id"`
	AgentID      string     `json:"agent_id"`
	Todos        []TodoItem `json:"todos"`
}

// TodoItem describes one sub-task within a TodoCreatedPayload.
// Ordinality is 1-based; DependsOn references ordinality values of
// prerequisite TodoItems in the same payload.
type TodoItem struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Instructions   string `json:"instructions"`         // full prompt for the developer agent
	Ordinality     int    `json:"ordinality"`           // 1-based position
	CanRunParallel bool   `json:"can_run_parallel"`     // true = no predecessor dependency
	DependsOn      []int  `json:"depends_on,omitempty"` // ordinality values that must complete first
}
