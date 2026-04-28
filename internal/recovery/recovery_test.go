package recovery

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// fakeDataManager is a minimal entitygraph.DataManager mock that captures
// the calls ReconcileRunningRuns makes and returns canned responses.
type fakeDataManager struct {
	listResp     []entitygraph.Entity
	listErr      error
	updateErrFor map[string]error // entityID -> err
	updates      []updateCall
	listFilter   entitygraph.EntityFilter
}

type updateCall struct {
	entityID string
	props    map[string]any
}

func (f *fakeDataManager) ListEntities(_ context.Context, filter entitygraph.EntityFilter) ([]entitygraph.Entity, error) {
	f.listFilter = filter
	return f.listResp, f.listErr
}

func (f *fakeDataManager) UpdateEntity(_ context.Context, _, entityID string, req entitygraph.UpdateEntityRequest) (entitygraph.Entity, error) {
	if err := f.updateErrFor[entityID]; err != nil {
		return entitygraph.Entity{}, err
	}
	f.updates = append(f.updates, updateCall{entityID: entityID, props: req.Properties})
	return entitygraph.Entity{ID: entityID, Properties: req.Properties}, nil
}

// Unused DataManager methods — return zero values so the struct satisfies the
// interface without affecting reconcile behaviour.
func (f *fakeDataManager) CreateEntity(context.Context, entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	return entitygraph.Entity{}, nil
}
func (f *fakeDataManager) GetEntity(context.Context, string, string) (entitygraph.Entity, error) {
	return entitygraph.Entity{}, nil
}
func (f *fakeDataManager) DeleteEntity(context.Context, string, string) error { return nil }
func (f *fakeDataManager) UpsertEntity(context.Context, entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	return entitygraph.Entity{}, nil
}
func (f *fakeDataManager) CreateRelationship(context.Context, entitygraph.CreateRelationshipRequest) (entitygraph.Relationship, error) {
	return entitygraph.Relationship{}, nil
}
func (f *fakeDataManager) GetRelationship(context.Context, string, string) (entitygraph.Relationship, error) {
	return entitygraph.Relationship{}, nil
}
func (f *fakeDataManager) DeleteRelationship(context.Context, string, string) error { return nil }
func (f *fakeDataManager) ListRelationships(context.Context, entitygraph.RelationshipFilter) ([]entitygraph.Relationship, error) {
	return nil, nil
}
func (f *fakeDataManager) TraverseGraph(context.Context, entitygraph.TraverseGraphRequest) (entitygraph.TraverseGraphResult, error) {
	return entitygraph.TraverseGraphResult{}, nil
}

// fakePublisher captures publish calls.
type fakePublisher struct {
	calls []string // topics
	err   error    // returned from Publish
}

func (p *fakePublisher) Publish(_ context.Context, topic string, _ string) error {
	p.calls = append(p.calls, topic)
	return p.err
}

func TestReconcileRunningRuns_NoRunningRuns(t *testing.T) {
	dm := &fakeDataManager{listResp: nil}
	pub := &fakePublisher{}

	if err := ReconcileRunningRuns(context.Background(), dm, pub, "agency-1", nil); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(dm.updates) != 0 {
		t.Errorf("expected no updates, got %d", len(dm.updates))
	}
	if len(pub.calls) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls))
	}
	if dm.listFilter.TypeID != "AgentRun" || dm.listFilter.Properties["status"] != "running" {
		t.Errorf("filter mismatch: %+v", dm.listFilter)
	}
}

func TestReconcileRunningRuns_FailsAndPublishes(t *testing.T) {
	dm := &fakeDataManager{listResp: []entitygraph.Entity{
		{ID: "run-1", TypeID: "AgentRun", Properties: map[string]any{"status": "running"}},
		{ID: "run-2", TypeID: "AgentRun", Properties: map[string]any{"status": "running"}},
	}}
	pub := &fakePublisher{}

	if err := ReconcileRunningRuns(context.Background(), dm, pub, "agency-1", nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(dm.updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(dm.updates))
	}
	for _, u := range dm.updates {
		if u.props["status"] != "failed" {
			t.Errorf("run %s status: got %v want failed", u.entityID, u.props["status"])
		}
		if u.props["error_message"] != "interrupted by service restart" {
			t.Errorf("run %s error_message: got %v", u.entityID, u.props["error_message"])
		}
	}

	wantTopic := "cross.ai.agency-1.run.failed"
	if len(pub.calls) != 2 {
		t.Fatalf("expected 2 publishes, got %d", len(pub.calls))
	}
	for _, c := range pub.calls {
		if c != wantTopic {
			t.Errorf("topic: got %q want %q", c, wantTopic)
		}
	}
}

func TestReconcileRunningRuns_PublisherFailureLoggedAndContinues(t *testing.T) {
	dm := &fakeDataManager{listResp: []entitygraph.Entity{
		{ID: "run-1", Properties: map[string]any{"status": "running"}},
		{ID: "run-2", Properties: map[string]any{"status": "running"}},
	}}
	pub := &fakePublisher{err: errors.New("cross down")}

	if err := ReconcileRunningRuns(context.Background(), dm, pub, "agency-1", nil); err != nil {
		t.Fatalf("publisher failure must not surface as error, got %v", err)
	}
	if len(dm.updates) != 2 {
		t.Errorf("both runs must still be marked failed despite publisher errors, got %d updates", len(dm.updates))
	}
}

func TestReconcileRunningRuns_NilPublisherSkipsPublish(t *testing.T) {
	dm := &fakeDataManager{listResp: []entitygraph.Entity{
		{ID: "run-1", Properties: map[string]any{"status": "running"}},
	}}

	if err := ReconcileRunningRuns(context.Background(), dm, nil, "agency-1", nil); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(dm.updates) != 1 {
		t.Errorf("expected 1 update, got %d", len(dm.updates))
	}
}

func TestReconcileRunningRuns_UpdateFailureSkipsPublishAndContinues(t *testing.T) {
	dm := &fakeDataManager{
		listResp: []entitygraph.Entity{
			{ID: "run-1", Properties: map[string]any{"status": "running"}},
			{ID: "run-2", Properties: map[string]any{"status": "running"}},
		},
		updateErrFor: map[string]error{"run-1": errors.New("write conflict")},
	}
	pub := &fakePublisher{}

	if err := ReconcileRunningRuns(context.Background(), dm, pub, "agency-1", nil); err != nil {
		t.Fatalf("err: %v", err)
	}
	// Only run-2 should produce an update and a publish; run-1's failure is logged.
	if len(dm.updates) != 1 || dm.updates[0].entityID != "run-2" {
		t.Errorf("expected single update for run-2, got %+v", dm.updates)
	}
	if len(pub.calls) != 1 {
		t.Errorf("expected 1 publish (skipped for failed update), got %d", len(pub.calls))
	}
}

func TestReconcileRunningRuns_ListErrorSurfaces(t *testing.T) {
	dm := &fakeDataManager{listErr: errors.New("graph down")}
	pub := &fakePublisher{}

	err := ReconcileRunningRuns(context.Background(), dm, pub, "agency-1", nil)
	if err == nil {
		t.Fatal("expected error from list failure")
	}
	if len(pub.calls) != 0 {
		t.Errorf("must not publish when list fails")
	}
}
