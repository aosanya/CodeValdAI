package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EventReceiverServer implements sharedev1.EventReceiverServiceServer.
// Cross calls NotifyEvent to push a subscribed event; the handler writes a
// ReceivedEvent record before returning so Cross can Ack the delivery.
// If dispatcher is non-nil, dispatch is fired asynchronously after the ACK.
type EventReceiverServer struct {
	sharedev1.UnimplementedEventReceiverServiceServer
	backend    entitygraph.DataManager
	agencyID   string
	dispatcher EventDispatcher
}

// NewEventReceiver constructs an EventReceiverServer.
// Pass a non-nil dispatcher to enable RACI-driven AgentRun triggering;
// pass nil to operate in store-only mode.
func NewEventReceiver(backend entitygraph.DataManager, agencyID string, dispatcher EventDispatcher) *EventReceiverServer {
	return &EventReceiverServer{backend: backend, agencyID: agencyID, dispatcher: dispatcher}
}

// NotifyEvent receives a pushed event from Cross.
// It deduplicates by event_id — if a ReceivedEvent with the same event_id
// already exists the delivery is ACKed immediately without re-dispatching.
// On a new event it writes the ReceivedEvent first; on DB failure it returns
// codes.Internal so Cross leaves the delivery in "pending" state.
func (s *EventReceiverServer) NotifyEvent(ctx context.Context, req *sharedev1.NotifyEventRequest) (*sharedev1.NotifyEventResponse, error) {
	eventID := req.GetEventId()

	// Idempotency check: ACK without re-dispatching if already processed.
	if eventID != "" {
		existing, err := s.backend.ListEntities(ctx, entitygraph.EntityFilter{
			AgencyID: s.agencyID,
			TypeID:   "ReceivedEvent",
			Properties: map[string]any{
				"event_id": eventID,
			},
		})
		if err == nil && len(existing) > 0 {
			log.Printf("codevaldai: NotifyEvent: duplicate event_id=%s topic=%s — skipping dispatch", eventID, req.GetTopic())
			return &sharedev1.NotifyEventResponse{}, nil
		}
	}

	_, err := s.backend.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: s.agencyID,
		TypeID:   "ReceivedEvent",
		Properties: map[string]any{
			"event_id":    eventID,
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
		eventID, req.GetTopic(), req.GetSource())

	if req.GetTopic() == "git.file.written" {
		go s.handleFileWritten(context.Background(), req.GetPayload())
	}

	if s.dispatcher != nil {
		go s.dispatcher.Dispatch(context.Background(), req.GetTopic(), req.GetPayload())
	}

	return &sharedev1.NotifyEventResponse{}, nil
}

// fileWrittenPayload mirrors the CodeValdGit FileWrittenPayload.
type fileWrittenPayload struct {
	RunID      string `json:"run_id"`
	Repository string `json:"repository"`
	BranchName string `json:"branch_name"`
	Path       string `json:"path"`
	CommitSHA  string `json:"commit_sha"`
}

// handleFileWritten updates the run debrief entry for the written file,
// replacing [dispatched] with [committed: <sha>] for the matching path.
func (s *EventReceiverServer) handleFileWritten(ctx context.Context, rawPayload string) {
	var p fileWrittenPayload
	if err := json.Unmarshal([]byte(rawPayload), &p); err != nil || p.RunID == "" {
		return
	}

	run, err := s.backend.GetEntity(ctx, s.agencyID, p.RunID)
	if err != nil {
		log.Printf("codevaldai: handleFileWritten: GetEntity run=%s: %v", p.RunID, err)
		return
	}

	current := ""
	if v, ok := run.Properties["debrief"]; ok {
		current, _ = v.(string)
	}
	if current == "" {
		return
	}

	// Replace the matching [dispatched] line for this path with [committed: sha].
	marker := fmt.Sprintf("path=`%s`", p.Path)
	updated := strings.ReplaceAll(
		current,
		marker+" branch=`"+p.BranchName+"` [dispatched]",
		marker+" branch=`"+p.BranchName+"` [committed: "+p.CommitSHA+"]",
	)
	if updated == current {
		return // nothing to update
	}

	if _, err := s.backend.UpdateEntity(ctx, s.agencyID, p.RunID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{"debrief": updated},
	}); err != nil {
		log.Printf("codevaldai: handleFileWritten: update debrief run=%s: %v", p.RunID, err)
	}
}
