package server

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldai "github.com/aosanya/CodeValdAI"
	pb "github.com/aosanya/CodeValdAI/gen/go/codevaldai/v1"
)

// TestRollbackByWorkflowRun_RPC_RoundTrip exercises the happy path: the handler
// forwards the request to the manager and translates the result into a
// RollbackByWorkflowRunResponse.
func TestRollbackByWorkflowRun_RPC_RoundTrip(t *testing.T) {
	mgr := &fakeAIManager{
		rollbackResult: codevaldai.RollbackByWorkflowRunResult{
			WorkflowRunID:    "wf-1",
			CancelledRunIDs:  []string{"run-a"},
			RolledBackRunIDs: []string{"run-b", "run-c"},
			SkippedRunIDs:    []string{"run-d"},
		},
	}
	srv := New(mgr)

	res, err := srv.RollbackByWorkflowRun(context.Background(), &pb.RollbackByWorkflowRunRequest{
		WorkflowRunId: "wf-1",
		Reason:        "regression",
	})
	if err != nil {
		t.Fatalf("RollbackByWorkflowRun: %v", err)
	}
	if res.WorkflowRunId != "wf-1" {
		t.Errorf("WorkflowRunId = %q want wf-1", res.WorkflowRunId)
	}
	if len(res.CancelledRunIds) != 1 || res.CancelledRunIds[0] != "run-a" {
		t.Errorf("CancelledRunIds = %v want [run-a]", res.CancelledRunIds)
	}
	if len(res.RolledBackRunIds) != 2 {
		t.Errorf("RolledBackRunIds len = %d want 2", len(res.RolledBackRunIds))
	}
	if len(res.SkippedRunIds) != 1 || res.SkippedRunIds[0] != "run-d" {
		t.Errorf("SkippedRunIds = %v want [run-d]", res.SkippedRunIds)
	}

	if len(mgr.rollbackCalls) != 1 {
		t.Fatalf("manager called %d times, want 1", len(mgr.rollbackCalls))
	}
	if mgr.rollbackCalls[0].workflowRunID != "wf-1" || mgr.rollbackCalls[0].reason != "regression" {
		t.Errorf("manager call args = %+v want {wf-1, regression}", mgr.rollbackCalls[0])
	}
}

// TestRollbackByWorkflowRun_RPC_EmptyID_InvalidArgument verifies the manager's
// ErrWorkflowRunIDRequired surfaces as gRPC INVALID_ARGUMENT.
func TestRollbackByWorkflowRun_RPC_EmptyID_InvalidArgument(t *testing.T) {
	mgr := &fakeAIManager{rollbackErr: codevaldai.ErrWorkflowRunIDRequired}
	srv := New(mgr)

	_, err := srv.RollbackByWorkflowRun(context.Background(), &pb.RollbackByWorkflowRunRequest{})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("err code = %v want InvalidArgument (err=%v)", got, err)
	}
}

// TestRollbackByWorkflowRun_RPC_ManagerError_Internal verifies non-domain
// errors surface as gRPC INTERNAL.
func TestRollbackByWorkflowRun_RPC_ManagerError_Internal(t *testing.T) {
	mgr := &fakeAIManager{rollbackErr: errors.New("storage went sideways")}
	srv := New(mgr)

	_, err := srv.RollbackByWorkflowRun(context.Background(), &pb.RollbackByWorkflowRunRequest{
		WorkflowRunId: "wf-x",
	})
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("err code = %v want Internal (err=%v)", got, err)
	}
}

// TestStatusConverters_RollbackStatuses verifies the new enum values
// round-trip through both directions of the status converter.
func TestStatusConverters_RollbackStatuses(t *testing.T) {
	cases := []struct {
		domain codevaldai.AgentRunStatus
		proto  pb.AgentRunStatus
	}{
		{codevaldai.AgentRunStatusCancelled, pb.AgentRunStatus_AGENT_RUN_STATUS_CANCELLED},
		{codevaldai.AgentRunStatusRolledBack, pb.AgentRunStatus_AGENT_RUN_STATUS_ROLLED_BACK},
	}
	for _, c := range cases {
		if got := domainStatusToProto(c.domain); got != c.proto {
			t.Errorf("domainStatusToProto(%q) = %v want %v", c.domain, got, c.proto)
		}
		if got := protoStatusToDomain(c.proto); got != c.domain {
			t.Errorf("protoStatusToDomain(%v) = %q want %q", c.proto, got, c.domain)
		}
	}
}
