// Package registrar provides the CodeValdAI service registrar.
// It wraps the shared-library heartbeat registrar and additionally implements
// [codevaldai.CrossPublisher] so the [AIManager] can notify CodeValdCross
// whenever an agent is created or a run completes.
package registrar

import (
	"context"
	"log"
	"time"

	codevaldai "github.com/aosanya/CodeValdAI"
	egserver "github.com/aosanya/CodeValdSharedLib/entitygraph/server"
	sharedregistrar "github.com/aosanya/CodeValdSharedLib/registrar"
	"github.com/aosanya/CodeValdSharedLib/schemaroutes"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// Registrar handles two responsibilities:
//  1. Sending periodic heartbeat registrations to CodeValdCross via the
//     shared-library registrar (Run / Close).
//  2. Implementing [codevaldai.CrossPublisher] so that AIManager can fire
//     cross-service events on successful agent and run operations.
//
// Construct via [New]; start heartbeats by calling Run in a goroutine; stop
// by cancelling the context then calling Close.
type Registrar struct {
	heartbeat sharedregistrar.Registrar
}

// Compile-time assertion that *Registrar implements CrossPublisher.
var _ codevaldai.CrossPublisher = (*Registrar)(nil)

// New constructs a Registrar that heartbeats to the CodeValdCross gRPC server
// at crossAddr and can publish AI lifecycle events.
//
//   - crossAddr     — host:port of the CodeValdCross gRPC server
//   - advertiseAddr — host:port that Cross dials back on
//   - agencyID      — agency scoped to this service instance
//   - pingInterval  — heartbeat cadence; \u2264 0 means only the initial ping
//   - pingTimeout   — per-RPC timeout for each Register call
func New(
	crossAddr, advertiseAddr, agencyID string,
	pingInterval, pingTimeout time.Duration,
) (*Registrar, error) {
	routes := aiRoutes()
	hb, err := sharedregistrar.New(
		crossAddr,
		advertiseAddr,
		agencyID,
		"codevaldai",
		[]string{
			"cross.ai.{agencyID}.agent.created",
			"cross.ai.{agencyID}.run.completed",
			"cross.ai.{agencyID}.run.failed",
		},
		[]string{"work.task.status.changed"},
		routes,
		pingInterval,
		pingTimeout,
	)
	if err != nil {
		return nil, err
	}
	return &Registrar{heartbeat: hb}, nil
}

// Run starts the heartbeat loop, sending an immediate Register ping to
// CodeValdCross then repeating at the configured interval until ctx is
// cancelled. Must be called inside a goroutine.
func (r *Registrar) Run(ctx context.Context) {
	r.heartbeat.Run(ctx)
}

// Close releases the underlying gRPC connection used for heartbeats.
// Call after the context passed to Run has been cancelled.
func (r *Registrar) Close() {
	r.heartbeat.Close()
}

// Publish implements [codevaldai.CrossPublisher].
// It fires a best-effort notification for topic and agencyID.
// Currently logs the event; a future iteration will call a Cross Publish RPC
// once CodeValdCross exposes one. Errors are always nil — the operation has
// already been persisted and must not be rolled back.
func (r *Registrar) Publish(ctx context.Context, topic string, agencyID string) error {
	log.Printf("registrar: publish topic=%q agencyID=%q", topic, agencyID)
	// TODO(CROSS-007): call OrchestratorService.Publish RPC when available.
	return nil
}

// aiRoutes returns all HTTP routes CodeValdAI exposes via Cross.
//
// It combines:
//   - Static routes for the AIService gRPC methods (providers, agents, runs).
//   - Dynamic entity CRUD routes generated from [codevaldai.DefaultAISchema]
//     via a single [schemaroutes.RoutesFromSchema] call.
func aiRoutes() []types.RouteInfo {
	static := []types.RouteInfo{
		// POST /ai/{agencyId}/providers — create a new LLMProvider.
		{
			Method:     "POST",
			Pattern:    "/ai/{agencyId}/providers",
			Capability: "create_provider",
			GrpcMethod: "/codevaldai.v1.AIService/CreateProvider",
		},
		// GET /ai/{agencyId}/providers — list all LLMProviders.
		{
			Method:     "GET",
			Pattern:    "/ai/{agencyId}/providers",
			Capability: "list_providers",
			GrpcMethod: "/codevaldai.v1.AIService/ListProviders",
		},
		// GET /ai/{agencyId}/providers/{providerId} — get a single LLMProvider.
		{
			Method:     "GET",
			Pattern:    "/ai/{agencyId}/providers/{providerId}",
			Capability: "get_provider",
			GrpcMethod: "/codevaldai.v1.AIService/GetProvider",
			PathBindings: []types.PathBinding{
				{URLParam: "providerId", Field: "provider_id"},
			},
		},
		// PUT /ai/{agencyId}/providers/{providerId} — update a LLMProvider.
		{
			Method:     "PUT",
			Pattern:    "/ai/{agencyId}/providers/{providerId}",
			Capability: "update_provider",
			GrpcMethod: "/codevaldai.v1.AIService/UpdateProvider",
			PathBindings: []types.PathBinding{
				{URLParam: "providerId", Field: "provider_id"},
			},
		},
		// DELETE /ai/{agencyId}/providers/{providerId} — delete a LLMProvider.
		{
			Method:     "DELETE",
			Pattern:    "/ai/{agencyId}/providers/{providerId}",
			Capability: "delete_provider",
			GrpcMethod: "/codevaldai.v1.AIService/DeleteProvider",
			PathBindings: []types.PathBinding{
				{URLParam: "providerId", Field: "provider_id"},
			},
		},
		// POST /ai/{agencyId}/agents — create a new Agent.
		{
			Method:     "POST",
			Pattern:    "/ai/{agencyId}/agents",
			Capability: "create_agent",
			GrpcMethod: "/codevaldai.v1.AIService/CreateAgent",
		},
		// GET /ai/{agencyId}/agents — list all Agents.
		{
			Method:     "GET",
			Pattern:    "/ai/{agencyId}/agents",
			Capability: "list_agents",
			GrpcMethod: "/codevaldai.v1.AIService/ListAgents",
		},
		// GET /ai/{agencyId}/agents/{agentId} — get a single Agent.
		{
			Method:     "GET",
			Pattern:    "/ai/{agencyId}/agents/{agentId}",
			Capability: "get_agent",
			GrpcMethod: "/codevaldai.v1.AIService/GetAgent",
			PathBindings: []types.PathBinding{
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		// PUT /ai/{agencyId}/agents/{agentId} — update an Agent.
		{
			Method:     "PUT",
			Pattern:    "/ai/{agencyId}/agents/{agentId}",
			Capability: "update_agent",
			GrpcMethod: "/codevaldai.v1.AIService/UpdateAgent",
			PathBindings: []types.PathBinding{
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		// DELETE /ai/{agencyId}/agents/{agentId} — delete an Agent.
		{
			Method:     "DELETE",
			Pattern:    "/ai/{agencyId}/agents/{agentId}",
			Capability: "delete_agent",
			GrpcMethod: "/codevaldai.v1.AIService/DeleteAgent",
			PathBindings: []types.PathBinding{
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		// POST /ai/{agencyId}/runs/intake — start the intake phase.
		{
			Method:     "POST",
			Pattern:    "/ai/{agencyId}/runs/intake",
			Capability: "intake_run",
			GrpcMethod: "/codevaldai.v1.AIService/IntakeRun",
		},
		// POST /ai/{agencyId}/runs/{runId}/execute — submit inputs and execute.
		{
			Method:     "POST",
			Pattern:    "/ai/{agencyId}/runs/{runId}/execute",
			Capability: "execute_run",
			GrpcMethod: "/codevaldai.v1.AIService/ExecuteRun",
			PathBindings: []types.PathBinding{
				{URLParam: "runId", Field: "run_id"},
			},
		},
		// POST /ai/{agencyId}/runs/{runId}/execute/stream — streaming execute.
		{
			Method:     "POST",
			Pattern:    "/ai/{agencyId}/runs/{runId}/execute/stream",
			Capability: "execute_run_streaming",
			GrpcMethod: "/codevaldai.v1.AIService/ExecuteRunStreaming",
			PathBindings: []types.PathBinding{
				{URLParam: "runId", Field: "run_id"},
			},
		},
		// GET /ai/{agencyId}/runs/{runId} — get a single AgentRun.
		{
			Method:     "GET",
			Pattern:    "/ai/{agencyId}/runs/{runId}",
			Capability: "get_run",
			GrpcMethod: "/codevaldai.v1.AIService/GetRun",
			PathBindings: []types.PathBinding{
				{URLParam: "runId", Field: "run_id"},
			},
		},
		// GET /ai/{agencyId}/runs — list runs (optionally filtered).
		{
			Method:     "GET",
			Pattern:    "/ai/{agencyId}/runs",
			Capability: "list_runs",
			GrpcMethod: "/codevaldai.v1.AIService/ListRuns",
		},
	}

	// Dynamic entity CRUD routes derived from the AI schema.
	dynamic := schemaroutes.RoutesFromSchema(
		codevaldai.DefaultAISchema(),
		"/ai/{agencyId}",
		"agencyId",
		egserver.GRPCServicePath,
	)

	return append(static, dynamic...)
}
