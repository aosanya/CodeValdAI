package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── fakeDataManager ───────────────────────────────────────────────────────────

type fakeDataManager struct {
	mu       sync.Mutex
	seq      int
	entities []entitygraph.CreateEntityRequest
	err      error // if set, CreateEntity returns this
}

func (f *fakeDataManager) nextID() string {
	f.seq++
	return fmt.Sprintf("fake-%d", f.seq)
}

func (f *fakeDataManager) CreateEntity(_ context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return entitygraph.Entity{}, f.err
	}
	f.entities = append(f.entities, req)
	return entitygraph.Entity{
		ID:         f.nextID(),
		AgencyID:   req.AgencyID,
		TypeID:     req.TypeID,
		Properties: req.Properties,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

func (f *fakeDataManager) UpsertEntity(ctx context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	return f.CreateEntity(ctx, req)
}
func (f *fakeDataManager) GetEntity(_ context.Context, _, _ string) (entitygraph.Entity, error) {
	return entitygraph.Entity{}, nil
}
func (f *fakeDataManager) UpdateEntity(_ context.Context, _, _ string, _ entitygraph.UpdateEntityRequest) (entitygraph.Entity, error) {
	return entitygraph.Entity{}, nil
}
func (f *fakeDataManager) DeleteEntity(_ context.Context, _, _ string) error { return nil }
func (f *fakeDataManager) ListEntities(_ context.Context, _ entitygraph.EntityFilter) ([]entitygraph.Entity, error) {
	return nil, nil
}
func (f *fakeDataManager) CreateRelationship(_ context.Context, _ entitygraph.CreateRelationshipRequest) (entitygraph.Relationship, error) {
	return entitygraph.Relationship{}, nil
}
func (f *fakeDataManager) GetRelationship(_ context.Context, _, _ string) (entitygraph.Relationship, error) {
	return entitygraph.Relationship{}, nil
}
func (f *fakeDataManager) DeleteRelationship(_ context.Context, _, _ string) error { return nil }
func (f *fakeDataManager) ListRelationships(_ context.Context, _ entitygraph.RelationshipFilter) ([]entitygraph.Relationship, error) {
	return nil, nil
}
func (f *fakeDataManager) TraverseGraph(_ context.Context, _ entitygraph.TraverseGraphRequest) (entitygraph.TraverseGraphResult, error) {
	return entitygraph.TraverseGraphResult{}, nil
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestNotifyEvent_WritesReceivedEventAndReturnsSuccess(t *testing.T) {
	dm := &fakeDataManager{}
	srv := NewEventReceiver(dm, "agency-1", nil)

	req := &sharedev1.NotifyEventRequest{
		EventId:  "evt-123",
		Topic:    "work.task.status.changed",
		AgencyId: "agency-1",
		Source:   "codevaldwork",
		Payload:  `{"task_id":"t1","from":"pending","to":"in_progress"}`,
	}

	resp, err := srv.NotifyEvent(context.Background(), req)
	if err != nil {
		t.Fatalf("NotifyEvent returned unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()
	if len(dm.entities) != 1 {
		t.Fatalf("expected 1 entity written, got %d", len(dm.entities))
	}
	got := dm.entities[0]
	if got.TypeID != "ReceivedEvent" {
		t.Errorf("TypeID = %q, want %q", got.TypeID, "ReceivedEvent")
	}
	if got.AgencyID != "agency-1" {
		t.Errorf("AgencyID = %q, want %q", got.AgencyID, "agency-1")
	}
	if got.Properties["event_id"] != "evt-123" {
		t.Errorf("event_id = %v, want %q", got.Properties["event_id"], "evt-123")
	}
	if got.Properties["topic"] != "work.task.status.changed" {
		t.Errorf("topic = %v, want %q", got.Properties["topic"], "work.task.status.changed")
	}
	if got.Properties["source"] != "codevaldwork" {
		t.Errorf("source = %v, want %q", got.Properties["source"], "codevaldwork")
	}
	if _, ok := got.Properties["received_at"]; !ok {
		t.Error("received_at property missing")
	}
}

func TestNotifyEvent_DBFailureReturnsInternalError(t *testing.T) {
	dm := &fakeDataManager{err: errors.New("arangodb unavailable")}
	srv := NewEventReceiver(dm, "agency-1", nil)

	req := &sharedev1.NotifyEventRequest{
		EventId: "evt-456",
		Topic:   "work.task.status.changed",
	}

	_, err := srv.NotifyEvent(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if s.Code() != codes.Internal {
		t.Errorf("code = %v, want %v", s.Code(), codes.Internal)
	}
}
