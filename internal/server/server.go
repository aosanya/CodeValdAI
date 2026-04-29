// Package server implements the AIService gRPC handler.
// It wraps a codevaldai.AIManager and translates between proto messages
// and domain types.
package server

import (
	"context"
	"time"

	codevaldai "github.com/aosanya/CodeValdAI"
	pb "github.com/aosanya/CodeValdAI/gen/go/codevaldai/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements pb.AIServiceServer by wrapping a codevaldai.AIManager.
// Construct via New; register with:
//
//	pb.RegisterAIServiceServer(grpcServer, server.New(mgr))
type Server struct {
	pb.UnimplementedAIServiceServer
	mgr codevaldai.AIManager
}

// New constructs a Server backed by the given AIManager.
func New(mgr codevaldai.AIManager) *Server {
	return &Server{mgr: mgr}
}

// ── Provider RPC handlers ────────────────────────────────────────────────────

// CreateProvider implements pb.AIServiceServer.
func (s *Server) CreateProvider(ctx context.Context, req *pb.CreateProviderRequest) (*pb.LLMProvider, error) {
	provider, err := s.mgr.CreateProvider(ctx, codevaldai.CreateProviderRequest{
		Name:         req.GetName(),
		ProviderType: req.GetProviderType(),
		APIKey:       req.GetApiKey(),
		BaseURL:      req.GetBaseUrl(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return providerToProto(provider), nil
}

// GetProvider implements pb.AIServiceServer.
func (s *Server) GetProvider(ctx context.Context, req *pb.GetProviderRequest) (*pb.LLMProvider, error) {
	provider, err := s.mgr.GetProvider(ctx, req.GetProviderId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return providerToProto(provider), nil
}

// ListProviders implements pb.AIServiceServer.
func (s *Server) ListProviders(ctx context.Context, _ *pb.ListProvidersRequest) (*pb.ListProvidersResponse, error) {
	providers, err := s.mgr.ListProviders(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	out := make([]*pb.LLMProvider, len(providers))
	for i, p := range providers {
		out[i] = providerToProto(p)
	}
	return &pb.ListProvidersResponse{Providers: out}, nil
}

// UpdateProvider implements pb.AIServiceServer.
func (s *Server) UpdateProvider(ctx context.Context, req *pb.UpdateProviderRequest) (*pb.LLMProvider, error) {
	provider, err := s.mgr.UpdateProvider(ctx, req.GetProviderId(), codevaldai.UpdateProviderRequest{
		Name:    req.GetName(),
		APIKey:  req.GetApiKey(),
		BaseURL: req.GetBaseUrl(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return providerToProto(provider), nil
}

// DeleteProvider implements pb.AIServiceServer.
func (s *Server) DeleteProvider(ctx context.Context, req *pb.DeleteProviderRequest) (*pb.DeleteProviderResponse, error) {
	if err := s.mgr.DeleteProvider(ctx, req.GetProviderId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &pb.DeleteProviderResponse{}, nil
}

// ── Agent RPC handlers ───────────────────────────────────────────────────────

// CreateAgent implements pb.AIServiceServer.
func (s *Server) CreateAgent(ctx context.Context, req *pb.CreateAgentRequest) (*pb.Agent, error) {
	agent, err := s.mgr.CreateAgent(ctx, codevaldai.CreateAgentRequest{
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		ProviderID:   req.GetProviderId(),
		Model:        req.GetModel(),
		SystemPrompt: req.GetSystemPrompt(),
		Temperature:  req.GetTemperature(),
		MaxTokens:    int(req.GetMaxTokens()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return agentToProto(agent), nil
}

// GetAgent implements pb.AIServiceServer.
func (s *Server) GetAgent(ctx context.Context, req *pb.GetAgentRequest) (*pb.Agent, error) {
	agent, err := s.mgr.GetAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return agentToProto(agent), nil
}

// ListAgents implements pb.AIServiceServer.
func (s *Server) ListAgents(ctx context.Context, _ *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	agents, err := s.mgr.ListAgents(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	out := make([]*pb.Agent, len(agents))
	for i, a := range agents {
		out[i] = agentToProto(a)
	}
	return &pb.ListAgentsResponse{Agents: out}, nil
}

// UpdateAgent implements pb.AIServiceServer.
func (s *Server) UpdateAgent(ctx context.Context, req *pb.UpdateAgentRequest) (*pb.Agent, error) {
	agent, err := s.mgr.UpdateAgent(ctx, req.GetAgentId(), codevaldai.UpdateAgentRequest{
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		ProviderID:   req.GetProviderId(),
		Model:        req.GetModel(),
		SystemPrompt: req.GetSystemPrompt(),
		Temperature:  req.GetTemperature(),
		MaxTokens:    int(req.GetMaxTokens()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return agentToProto(agent), nil
}

// DeleteAgent implements pb.AIServiceServer.
func (s *Server) DeleteAgent(ctx context.Context, req *pb.DeleteAgentRequest) (*pb.DeleteAgentResponse, error) {
	if err := s.mgr.DeleteAgent(ctx, req.GetAgentId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &pb.DeleteAgentResponse{}, nil
}

// ── Run RPC handlers ─────────────────────────────────────────────────────────

// IntakeRun implements pb.AIServiceServer.
func (s *Server) IntakeRun(ctx context.Context, req *pb.IntakeRunRequest) (*pb.IntakeRunResponse, error) {
	run, fields, err := s.mgr.IntakeRun(ctx, codevaldai.IntakeRunRequest{
		AgentID:      req.GetAgentId(),
		Instructions: req.GetInstructions(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	pbFields := make([]*pb.RunField, len(fields))
	for i, f := range fields {
		pbFields[i] = runFieldToProto(f)
	}
	return &pb.IntakeRunResponse{Run: agentRunToProto(run), Fields: pbFields}, nil
}

// ExecuteRun implements pb.AIServiceServer.
func (s *Server) ExecuteRun(ctx context.Context, req *pb.ExecuteRunRequest) (*pb.AgentRun, error) {
	run, err := s.mgr.ExecuteRun(ctx, req.GetRunId(), toRunInputs(req.GetInputs()))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return agentRunToProto(run), nil
}

// ExecuteRunStreaming implements pb.AIServiceServer.
// Forwards LLM output chunks to the client as they arrive, then sends the
// terminal AgentRun as the final stream message. Send failures on individual
// chunks are logged and do not abort the LLM call — the run always reaches a
// terminal state regardless of stream health.
func (s *Server) ExecuteRunStreaming(req *pb.ExecuteRunRequest, stream pb.AIService_ExecuteRunStreamingServer) error {
	onChunk := func(chunk string) {
		_ = stream.Send(&pb.ExecuteRunStreamingResponse{
			Payload: &pb.ExecuteRunStreamingResponse_Chunk{Chunk: chunk},
		})
	}
	run, err := s.mgr.ExecuteRunStreaming(stream.Context(), req.GetRunId(), toRunInputs(req.GetInputs()), onChunk)
	if err != nil {
		return toGRPCError(err)
	}
	return stream.Send(&pb.ExecuteRunStreamingResponse{
		Payload: &pb.ExecuteRunStreamingResponse_Run{Run: agentRunToProto(run)},
	})
}

// GetRun implements pb.AIServiceServer.
func (s *Server) GetRun(ctx context.Context, req *pb.GetRunRequest) (*pb.AgentRun, error) {
	run, err := s.mgr.GetRun(ctx, req.GetRunId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return agentRunToProto(run), nil
}

// ListRuns implements pb.AIServiceServer.
func (s *Server) ListRuns(ctx context.Context, req *pb.ListRunsRequest) (*pb.ListRunsResponse, error) {
	filter := codevaldai.RunFilter{
		AgentID: req.GetAgentId(),
	}
	if req.GetStatus() != pb.AgentRunStatus_AGENT_RUN_STATUS_UNSPECIFIED {
		filter.Status = protoStatusToDomain(req.GetStatus())
	}
	runs, err := s.mgr.ListRuns(ctx, filter)
	if err != nil {
		return nil, toGRPCError(err)
	}
	out := make([]*pb.AgentRun, len(runs))
	for i, r := range runs {
		out[i] = agentRunToProto(r)
	}
	return &pb.ListRunsResponse{Runs: out}, nil
}

// ── Domain → Proto converters ────────────────────────────────────────────────

func providerToProto(p codevaldai.LLMProvider) *pb.LLMProvider {
	return &pb.LLMProvider{
		Id:           p.ID,
		Name:         p.Name,
		ProviderType: p.ProviderType,
		ApiKey:       p.APIKey,
		BaseUrl:      p.BaseURL,
		CreatedAt:    parseTimestamp(p.CreatedAt),
		UpdatedAt:    parseTimestamp(p.UpdatedAt),
	}
}

func agentToProto(a codevaldai.Agent) *pb.Agent {
	return &pb.Agent{
		Id:           a.ID,
		Name:         a.Name,
		Description:  a.Description,
		ProviderId:   a.ProviderID,
		Model:        a.Model,
		SystemPrompt: a.SystemPrompt,
		Temperature:  a.Temperature,
		MaxTokens:    int32(a.MaxTokens),
		CreatedAt:    parseTimestamp(a.CreatedAt),
		UpdatedAt:    parseTimestamp(a.UpdatedAt),
	}
}

func agentRunToProto(r codevaldai.AgentRun) *pb.AgentRun {
	return &pb.AgentRun{
		Id:           r.ID,
		AgentId:      r.AgentID,
		Instructions: r.Instructions,
		Status:       domainStatusToProto(r.Status),
		Output:       r.Output,
		ErrorMessage: r.ErrorMessage,
		InputTokens:  int32(r.InputTokens),
		OutputTokens: int32(r.OutputTokens),
		StartedAt:    parseTimestamp(r.StartedAt),
		CompletedAt:  parseTimestamp(r.CompletedAt),
		CreatedAt:    parseTimestamp(r.CreatedAt),
		UpdatedAt:    parseTimestamp(r.UpdatedAt),
	}
}

func runFieldToProto(f codevaldai.RunField) *pb.RunField {
	return &pb.RunField{
		Id:        f.ID,
		Fieldname: f.Fieldname,
		Type:      f.Type,
		Label:     f.Label,
		Required:  f.Required,
		Options:   f.Options,
		Ordinality: int32(f.Ordinality),
	}
}

func toRunInputs(pbInputs []*pb.RunInput) []codevaldai.RunInput {
	inputs := make([]codevaldai.RunInput, len(pbInputs))
	for i, inp := range pbInputs {
		inputs[i] = codevaldai.RunInput{Fieldname: inp.GetFieldname(), Value: inp.GetValue()}
	}
	return inputs
}

func domainStatusToProto(s codevaldai.AgentRunStatus) pb.AgentRunStatus {
	switch s {
	case codevaldai.AgentRunStatusPendingIntake:
		return pb.AgentRunStatus_AGENT_RUN_STATUS_PENDING_INTAKE
	case codevaldai.AgentRunStatusPendingExecution:
		return pb.AgentRunStatus_AGENT_RUN_STATUS_PENDING_EXECUTION
	case codevaldai.AgentRunStatusRunning:
		return pb.AgentRunStatus_AGENT_RUN_STATUS_RUNNING
	case codevaldai.AgentRunStatusCompleted:
		return pb.AgentRunStatus_AGENT_RUN_STATUS_COMPLETED
	case codevaldai.AgentRunStatusFailed:
		return pb.AgentRunStatus_AGENT_RUN_STATUS_FAILED
	default:
		return pb.AgentRunStatus_AGENT_RUN_STATUS_UNSPECIFIED
	}
}

func protoStatusToDomain(s pb.AgentRunStatus) codevaldai.AgentRunStatus {
	switch s {
	case pb.AgentRunStatus_AGENT_RUN_STATUS_PENDING_INTAKE:
		return codevaldai.AgentRunStatusPendingIntake
	case pb.AgentRunStatus_AGENT_RUN_STATUS_PENDING_EXECUTION:
		return codevaldai.AgentRunStatusPendingExecution
	case pb.AgentRunStatus_AGENT_RUN_STATUS_RUNNING:
		return codevaldai.AgentRunStatusRunning
	case pb.AgentRunStatus_AGENT_RUN_STATUS_COMPLETED:
		return codevaldai.AgentRunStatusCompleted
	case pb.AgentRunStatus_AGENT_RUN_STATUS_FAILED:
		return codevaldai.AgentRunStatusFailed
	default:
		return ""
	}
}

func parseTimestamp(s string) *timestamppb.Timestamp {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return timestamppb.New(t)
}
