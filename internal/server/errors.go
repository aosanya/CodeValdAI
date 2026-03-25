// Package server implements the AIService gRPC handler.
package server

import (
	"errors"

	codevaldai "github.com/aosanya/CodeValdAI"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toGRPCError maps CodeValdAI domain errors to the appropriate gRPC status.
// Unknown errors are wrapped as codes.Internal.
func toGRPCError(err error) error {
	switch {
	case errors.Is(err, codevaldai.ErrProviderNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, codevaldai.ErrAgentNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, codevaldai.ErrRunNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, codevaldai.ErrInvalidProvider):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, codevaldai.ErrInvalidAgent):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, codevaldai.ErrInvalidLLMResponse):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, codevaldai.ErrProviderInUse):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, codevaldai.ErrAgentHasActiveRuns):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, codevaldai.ErrRunNotIntaked):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, codevaldai.ErrInvalidRunStatus):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "internal error: %v", err)
	}
}
