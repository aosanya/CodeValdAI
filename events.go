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

	// Planner dispatch topics — relayed on behalf of the task-planner agent.
	// These are work-domain topics; the AI acts as a relay, not the originator.
	// task.plan.split: planner breaks the task into child Tasks via CodeValdWork.
	// task.plan.decompose: planner signals a re-dispatch to the developer agent.
	TopicTaskPlanSplit    = "task.plan.split"
	TopicTaskPlanDecompose = "task.plan.decompose"

	// Rollback (WorkflowRun rollback coordinator — FEAT-20260602-004 Phase 2)
	// TopicRunCancelled is published once per in-flight AgentRun the rollback
	// coordinator cancels. Recipients should treat the run as terminal.
	TopicRunCancelled = "ai.run.cancelled"
	// TopicRunRolledBack is published once per already-terminal AgentRun the
	// rollback coordinator freezes as audit. The payload includes the original
	// terminal status so consumers can distinguish completed-then-rolled-back
	// from failed-then-rolled-back.
	TopicRunRolledBack = "ai.run.rolled_back"
)

// TaskStartedPayload is published when ExecuteRunStreaming transitions to
// the running state (before the LLM call). Signals that work has begun.
type TaskStartedPayload struct {
	TaskID        string
	RunID         string
	AgentID       string
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskCompletedPayload is published when ExecuteRunStreaming finishes
// successfully and actions have been dispatched.
//
// EmittedWrites carries the list of paths the run wrote via git.file.write
// actions during the same dispatch. CodeValdWork uses it to hold
// work.todo.completed until every emitted write has been confirmed by a
// matching git.file.written event (BUG-09-020 Phase 2).
type TaskCompletedPayload struct {
	TaskID        string
	RunID         string
	AgentID       string
	HasSubtasks   bool     `json:"has_subtasks,omitempty"`
	EmittedWrites []string `json:"emitted_writes,omitempty"`
	WorkflowRunID string   `json:"workflow_run_id,omitempty"`
}

// TaskFailedPayload is published when the LLM call errors, times out, or
// the output contains no actions block.
type TaskFailedPayload struct {
	TaskID        string
	RunID         string
	Reason        string
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
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
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TodoCreatedPayload is published on ai.todo.created when a developer agent
// decomposes an inbound task into sub-tasks.
type TodoCreatedPayload struct {
	ParentTaskID  string     `json:"parent_task_id"` // Work task that triggered the decomposition
	RunID         string     `json:"run_id"`
	AgentID       string     `json:"agent_id"`
	WorkflowRunID string     `json:"workflow_run_id,omitempty"`
	Todos         []TodoItem `json:"todos"`
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

// RunCancelledPayload is published on [TopicRunCancelled] once per in-flight
// AgentRun the rollback coordinator transitions to cancelled.
type RunCancelledPayload struct {
	RunID         string         `json:"run_id"`
	AgentID       string         `json:"agent_id,omitempty"`
	WorkflowRunID string         `json:"workflow_run_id"`
	PreviousStatus AgentRunStatus `json:"previous_status"`
	Reason        string         `json:"reason,omitempty"`
}

// RunRolledBackPayload is published on [TopicRunRolledBack] once per
// completed-or-failed AgentRun the rollback coordinator freezes as audit.
type RunRolledBackPayload struct {
	RunID         string         `json:"run_id"`
	AgentID       string         `json:"agent_id,omitempty"`
	WorkflowRunID string         `json:"workflow_run_id"`
	PreviousStatus AgentRunStatus `json:"previous_status"`
	Reason        string         `json:"reason,omitempty"`
}
