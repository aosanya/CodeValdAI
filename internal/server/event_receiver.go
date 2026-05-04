package server

import (
	"context"
	"log"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EventReceiverServer implements sharedev1.EventReceiverServiceServer.
// Cross calls NotifyEvent to push a subscribed event; the handler writes a
// ReceivedEvent record before returning so Cross can Ack the delivery.
type EventReceiverServer struct {
	sharedev1.UnimplementedEventReceiverServiceServer
	backend  entitygraph.DataManager
	agencyID string
}

// NewEventReceiver constructs an EventReceiverServer.
func NewEventReceiver(backend entitygraph.DataManager, agencyID string) *EventReceiverServer {
	return &EventReceiverServer{backend: backend, agencyID: agencyID}
}

// NotifyEvent receives a pushed event from Cross.
// It writes a ReceivedEvent record first; on DB failure it returns
// codes.Internal so Cross leaves the delivery in "pending" state.
func (s *EventReceiverServer) NotifyEvent(ctx context.Context, req *sharedev1.NotifyEventRequest) (*sharedev1.NotifyEventResponse, error) {
	_, err := s.backend.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: s.agencyID,
		TypeID:   "ReceivedEvent",
		Properties: map[string]any{
			"event_id":    req.GetEventId(),
			"topic":       req.GetTopic(),
			"agency_id":   req.GetAgencyId(),
			"source":      req.GetSource(),
			"payload":     req.GetPayload(),
			"received_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		log.Printf("codevaldai: NotifyEvent: write ReceivedEvent: %v", err)
		return nil, status.Errorf(codes.Internal, "log received event: %v", err)
	}
	log.Printf("codevaldai: NotifyEvent: ACK event_id=%s topic=%s source=%s",
		req.GetEventId(), req.GetTopic(), req.GetSource())
	return &sharedev1.NotifyEventResponse{}, nil
}
