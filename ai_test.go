package codevaldai

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/types"
)

const testAgencyID = "test-agency"

// ── fakeDataManager ───────────────────────────────────────────────────────────

type fakeDataManager struct {
	mu            sync.RWMutex
	entities      map[string]entitygraph.Entity
	relationships map[string]entitygraph.Relationship
	seq           int
}

func newFakeDM() *fakeDataManager {
	return &fakeDataManager{
		entities:      make(map[string]entitygraph.Entity),
		relationships: make(map[string]entitygraph.Relationship),
	}
}

func (f *fakeDataManager) nextID() string {
	f.seq++
	return fmt.Sprintf("fake-%d", f.seq)
}

func (f *fakeDataManager) CreateEntity(_ context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID()
	e := entitygraph.Entity{
		ID:         id,
		AgencyID:   req.AgencyID,
		TypeID:     req.TypeID,
		Properties: req.Properties,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	f.entities[id] = e
	for _, rel := range req.Relationships {
		rid := f.nextID()
		f.relationships[rid] = entitygraph.Relationship{
			ID:       rid,
			AgencyID: req.AgencyID,
			Name:     rel.Name,
			FromID:   id,
			ToID:     rel.ToID,
		}
	}
	return e, nil
}

func (f *fakeDataManager) GetEntity(_ context.Context, _, entityID string) (entitygraph.Entity, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	e, ok := f.entities[entityID]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	return e, nil
}

func (f *fakeDataManager) UpdateEntity(_ context.Context, _, entityID string, req entitygraph.UpdateEntityRequest) (entitygraph.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entities[entityID]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	if e.Properties == nil {
		e.Properties = make(map[string]any)
	}
	for k, v := range req.Properties {
		e.Properties[k] = v
	}
	e.UpdatedAt = time.Now()
	f.entities[entityID] = e
	return e, nil
}

func (f *fakeDataManager) DeleteEntity(_ context.Context, _, entityID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entities[entityID]
	if !ok || e.Deleted {
		return entitygraph.ErrEntityNotFound
	}
	now := time.Now()
	e.Deleted = true
	e.DeletedAt = &now
	f.entities[entityID] = e
	return nil
}

func (f *fakeDataManager) ListEntities(_ context.Context, filter entitygraph.EntityFilter) ([]entitygraph.Entity, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var out []entitygraph.Entity
	for _, e := range f.entities {
		if e.Deleted {
			continue
		}
		if filter.AgencyID != "" && e.AgencyID != filter.AgencyID {
			continue
		}
		if filter.TypeID != "" && e.TypeID != filter.TypeID {
			continue
		}
		match := true
		for k, v := range filter.Properties {
			if e.Properties[k] != v {
				match = false
				break
			}
		}
		if match {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeDataManager) UpsertEntity(ctx context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	return f.CreateEntity(ctx, req)
}

func (f *fakeDataManager) CreateRelationship(_ context.Context, req entitygraph.CreateRelationshipRequest) (entitygraph.Relationship, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rid := f.nextID()
	r := entitygraph.Relationship{
		ID:       rid,
		AgencyID: req.AgencyID,
		Name:     req.Name,
		FromID:   req.FromID,
		ToID:     req.ToID,
	}
	f.relationships[rid] = r
	return r, nil
}

func (f *fakeDataManager) GetRelationship(_ context.Context, _, relationshipID string) (entitygraph.Relationship, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	r, ok := f.relationships[relationshipID]
	if !ok {
		return entitygraph.Relationship{}, entitygraph.ErrRelationshipNotFound
	}
	return r, nil
}

func (f *fakeDataManager) DeleteRelationship(_ context.Context, _, relationshipID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.relationships[relationshipID]; !ok {
		return entitygraph.ErrRelationshipNotFound
	}
	delete(f.relationships, relationshipID)
	return nil
}

func (f *fakeDataManager) ListRelationships(_ context.Context, filter entitygraph.RelationshipFilter) ([]entitygraph.Relationship, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var out []entitygraph.Relationship
	for _, r := range f.relationships {
		if filter.AgencyID != "" && r.AgencyID != filter.AgencyID {
			continue
		}
		if filter.FromID != "" && r.FromID != filter.FromID {
			continue
		}
		if filter.ToID != "" && r.ToID != filter.ToID {
			continue
		}
		if filter.Name != "" && r.Name != filter.Name {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeDataManager) TraverseGraph(_ context.Context, _ entitygraph.TraverseGraphRequest) (entitygraph.TraverseGraphResult, error) {
	return entitygraph.TraverseGraphResult{}, nil
}

// ── fakeSchemaManager ─────────────────────────────────────────────────────────

type fakeSchemaManager struct{}

func (fakeSchemaManager) SetSchema(_ context.Context, _ types.Schema) error            { return nil }
func (fakeSchemaManager) GetSchema(_ context.Context, _ string) (types.Schema, error)   { return types.Schema{}, nil }
func (fakeSchemaManager) Publish(_ context.Context, _ string) error                     { return nil }
func (fakeSchemaManager) Activate(_ context.Context, _ string, _ int) error             { return nil }
func (fakeSchemaManager) GetActive(_ context.Context, _ string) (types.Schema, error)   { return types.Schema{}, nil }
func (fakeSchemaManager) GetVersion(_ context.Context, _ string, _ int) (types.Schema, error) {
	return types.Schema{}, nil
}
func (fakeSchemaManager) ListVersions(_ context.Context, _ string) ([]types.Schema, error) {
	return nil, nil
}

// ── fakePublisher ─────────────────────────────────────────────────────────────

type fakePublisher struct {
	mu     sync.Mutex
	topics []string
}

func (f *fakePublisher) Publish(_ context.Context, topic, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.topics = append(f.topics, topic)
	return nil
}

func (f *fakePublisher) published() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.topics))
	copy(cp, f.topics)
	return cp
}

// ── test helpers ──────────────────────────────────────────────────────────────

func newTestManager(dm *fakeDataManager) (AIManager, *fakePublisher) {
	pub := &fakePublisher{}
	return NewAIManager(dm, fakeSchemaManager{}, pub, testAgencyID), pub
}

// seedAgentWithProvider adds a LLMProvider and Agent directly into the fake DM
// and wires the uses_provider edge. Returns (providerID, agentID).
func seedAgentWithProvider(t *testing.T, dm *fakeDataManager, providerBaseURL string) (string, string) {
	t.Helper()
	dm.mu.Lock()
	defer dm.mu.Unlock()

	providerID := dm.nextID()
	dm.entities[providerID] = entitygraph.Entity{
		ID:       providerID,
		AgencyID: testAgencyID,
		TypeID:   "LLMProvider",
		Properties: map[string]any{
			"name":          "Test Provider",
			"provider_type": "openai",
			"api_key":       "test-key",
			"base_url":      providerBaseURL,
		},
	}

	agentID := dm.nextID()
	dm.entities[agentID] = entitygraph.Entity{
		ID:       agentID,
		AgencyID: testAgencyID,
		TypeID:   "Agent",
		Properties: map[string]any{
			"name":          "Test Agent",
			"model":         "gpt-4",
			"system_prompt": "you are a test agent",
		},
	}

	relID := dm.nextID()
	dm.relationships[relID] = entitygraph.Relationship{
		ID:       relID,
		AgencyID: testAgencyID,
		Name:     "uses_provider",
		FromID:   agentID,
		ToID:     providerID,
	}

	return providerID, agentID
}

// seedRunInPendingIntake inserts an AgentRun in pending_intake with a
// belongs_to_agent edge to agentID. Returns the run ID.
func seedRunInPendingIntake(t *testing.T, dm *fakeDataManager, agentID string) string {
	t.Helper()
	dm.mu.Lock()
	defer dm.mu.Unlock()

	runID := dm.nextID()
	dm.entities[runID] = entitygraph.Entity{
		ID:       runID,
		AgencyID: testAgencyID,
		TypeID:   "AgentRun",
		Properties: map[string]any{
			"instructions": "test instructions",
			"status":       string(AgentRunStatusPendingIntake),
		},
	}
	relID := dm.nextID()
	dm.relationships[relID] = entitygraph.Relationship{
		ID:       relID,
		AgencyID: testAgencyID,
		Name:     "belongs_to_agent",
		FromID:   runID,
		ToID:     agentID,
	}
	return runID
}

// ── Provider CRUD Tests ───────────────────────────────────────────────────────

func TestCreateProvider_MissingName(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		ProviderType: "anthropic", APIKey: "key",
	})
	if !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("got %v, want ErrInvalidProvider", err)
	}
}

func TestCreateProvider_MissingAPIKey(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "anthropic",
	})
	if !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("got %v, want ErrInvalidProvider", err)
	}
}

func TestCreateProvider_UnknownType(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "foobar", APIKey: "k",
	})
	if !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("got %v, want ErrInvalidProvider", err)
	}
}

func TestCreateProvider_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, err := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "Anthropic Test", ProviderType: "anthropic", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("provider ID must be non-empty")
	}
	if p.Name != "Anthropic Test" || p.ProviderType != "anthropic" || p.APIKey != "sk-test" {
		t.Fatalf("unexpected provider fields: %+v", p)
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.GetProvider(context.Background(), "no-such-id")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("got %v, want ErrProviderNotFound", err)
	}
}

func TestGetProvider_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	created, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "openai", APIKey: "k",
	})
	got, err := mgr.GetProvider(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID {
		t.Fatalf("ID mismatch: got %s want %s", got.ID, created.ID)
	}
}

func TestListProviders_Empty(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	providers, err := mgr.ListProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(providers))
	}
}

func TestListProviders_ReturnsAll(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	for i := 0; i < 3; i++ {
		mgr.CreateProvider(context.Background(), CreateProviderRequest{
			Name: fmt.Sprintf("p%d", i), ProviderType: "openai", APIKey: "k",
		})
	}
	providers, _ := mgr.ListProviders(context.Background())
	if len(providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(providers))
	}
}

func TestUpdateProvider_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.UpdateProvider(context.Background(), "no-such", UpdateProviderRequest{Name: "new"})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("got %v, want ErrProviderNotFound", err)
	}
}

func TestUpdateProvider_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	created, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "old", ProviderType: "openai", APIKey: "k",
	})
	updated, err := mgr.UpdateProvider(context.Background(), created.ID, UpdateProviderRequest{Name: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "new" {
		t.Fatalf("expected name %q, got %q", "new", updated.Name)
	}
}

func TestDeleteProvider_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	if err := mgr.DeleteProvider(context.Background(), "no-such"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("got %v, want ErrProviderNotFound", err)
	}
}

func TestDeleteProvider_InUse(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "openai", APIKey: "k",
	})
	mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "a", ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp",
	})
	if err := mgr.DeleteProvider(context.Background(), p.ID); !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("got %v, want ErrProviderInUse", err)
	}
}

func TestDeleteProvider_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "openai", APIKey: "k",
	})
	if err := mgr.DeleteProvider(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GetProvider(context.Background(), p.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound after delete, got %v", err)
	}
}

// ── Agent CRUD Tests ──────────────────────────────────────────────────────────

func TestCreateAgent_MissingRequiredFields(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "openai", APIKey: "k",
	})
	cases := []CreateAgentRequest{
		{ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp"},      // missing Name
		{Name: "a", Model: "gpt-4", SystemPrompt: "sp"},              // missing ProviderID
		{Name: "a", ProviderID: p.ID, SystemPrompt: "sp"},            // missing Model
		{Name: "a", ProviderID: p.ID, Model: "gpt-4"},                // missing SystemPrompt
	}
	for _, req := range cases {
		if _, err := mgr.CreateAgent(context.Background(), req); !errors.Is(err, ErrInvalidAgent) {
			t.Fatalf("req %+v: got %v, want ErrInvalidAgent", req, err)
		}
	}
}

func TestCreateAgent_ProviderNotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	_, err := mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "a", ProviderID: "no-such", Model: "gpt-4", SystemPrompt: "sp",
	})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("got %v, want ErrProviderNotFound", err)
	}
}

func TestCreateAgent_Success_PublishesEvent(t *testing.T) {
	mgr, pub := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{
		Name: "p", ProviderType: "openai", APIKey: "k",
	})
	a, err := mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "MyAgent", ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "you are helpful",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" {
		t.Fatal("agent ID must be non-empty")
	}
	if a.ProviderID != p.ID {
		t.Fatalf("ProviderID: got %q want %q", a.ProviderID, p.ID)
	}
	topics := pub.published()
	want := fmt.Sprintf("cross.ai.%s.agent.created", testAgencyID)
	if len(topics) != 1 || topics[0] != want {
		t.Fatalf("expected topic %q, got %v", want, topics)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	if _, err := mgr.GetAgent(context.Background(), "no-such"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestGetAgent_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{Name: "p", ProviderType: "openai", APIKey: "k"})
	created, _ := mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "a", ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp",
	})
	got, err := mgr.GetAgent(context.Background(), created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("GetAgent: err=%v id=%s want=%s", err, got.ID, created.ID)
	}
}

func TestListAgents_ReturnsAll(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{Name: "p", ProviderType: "openai", APIKey: "k"})
	for i := 0; i < 3; i++ {
		mgr.CreateAgent(context.Background(), CreateAgentRequest{
			Name: fmt.Sprintf("a%d", i), ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp",
		})
	}
	agents, _ := mgr.ListAgents(context.Background())
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}
}

func TestUpdateAgent_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	if _, err := mgr.UpdateAgent(context.Background(), "no-such", UpdateAgentRequest{Name: "new"}); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestUpdateAgent_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{Name: "p", ProviderType: "openai", APIKey: "k"})
	a, _ := mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "old", ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp",
	})
	updated, err := mgr.UpdateAgent(context.Background(), a.ID, UpdateAgentRequest{Name: "new"})
	if err != nil || updated.Name != "new" {
		t.Fatalf("UpdateAgent: err=%v name=%s", err, updated.Name)
	}
}

func TestDeleteAgent_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	if err := mgr.DeleteAgent(context.Background(), "no-such"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestDeleteAgent_HasActiveRuns(t *testing.T) {
	dm := newFakeDM()
	mgr, _ := newTestManager(dm)
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{Name: "p", ProviderType: "openai", APIKey: "k"})
	a, _ := mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "a", ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp",
	})
	// Insert an active run directly so DeleteAgent sees it.
	dm.mu.Lock()
	runID := dm.nextID()
	dm.entities[runID] = entitygraph.Entity{
		ID: runID, AgencyID: testAgencyID, TypeID: "AgentRun",
		Properties: map[string]any{"status": string(AgentRunStatusRunning)},
	}
	dm.mu.Unlock()
	if err := mgr.DeleteAgent(context.Background(), a.ID); !errors.Is(err, ErrAgentHasActiveRuns) {
		t.Fatalf("got %v, want ErrAgentHasActiveRuns", err)
	}
}

func TestDeleteAgent_Success(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	p, _ := mgr.CreateProvider(context.Background(), CreateProviderRequest{Name: "p", ProviderType: "openai", APIKey: "k"})
	a, _ := mgr.CreateAgent(context.Background(), CreateAgentRequest{
		Name: "a", ProviderID: p.ID, Model: "gpt-4", SystemPrompt: "sp",
	})
	if err := mgr.DeleteAgent(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GetAgent(context.Background(), a.ID); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound after delete, got %v", err)
	}
}

// ── GetRun / ListRuns Tests ───────────────────────────────────────────────────

func TestGetRun_NotFound(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	if _, err := mgr.GetRun(context.Background(), "no-such"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("got %v, want ErrRunNotFound", err)
	}
}

func TestListRuns_Empty(t *testing.T) {
	mgr, _ := newTestManager(newFakeDM())
	runs, err := mgr.ListRuns(context.Background(), RunFilter{})
	if err != nil || len(runs) != 0 {
		t.Fatalf("expected empty list, got err=%v count=%d", err, len(runs))
	}
}
