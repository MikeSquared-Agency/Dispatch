package broker

import (
	"context"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/DarlingtonDeveloper/Dispatch/internal/alexandria"
	"github.com/DarlingtonDeveloper/Dispatch/internal/config"
	"github.com/DarlingtonDeveloper/Dispatch/internal/forge"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
	"github.com/DarlingtonDeveloper/Dispatch/internal/warren"
)

// Mock implementations

type mockStore struct {
	tasks  map[uuid.UUID]*store.Task
	events []*store.TaskEvent
}

func newMockStore() *mockStore {
	return &mockStore{tasks: make(map[uuid.UUID]*store.Task)}
}

func (m *mockStore) CreateTask(_ context.Context, t *store.Task) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now()
	m.tasks[t.ID] = t
	return nil
}
func (m *mockStore) GetTask(_ context.Context, id uuid.UUID) (*store.Task, error) {
	return m.tasks[id], nil
}
func (m *mockStore) ListTasks(_ context.Context, _ store.TaskFilter) ([]*store.Task, error) {
	var out []*store.Task
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}
func (m *mockStore) UpdateTask(_ context.Context, t *store.Task) error {
	m.tasks[t.ID] = t
	return nil
}
func (m *mockStore) GetPendingTasks(_ context.Context) ([]*store.Task, error) {
	var out []*store.Task
	for _, t := range m.tasks {
		if t.Status == store.StatusPending {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *mockStore) GetRunningTasksForAgent(_ context.Context, agentID string) ([]*store.Task, error) {
	var out []*store.Task
	for _, t := range m.tasks {
		if t.Assignee == agentID && (t.Status == store.StatusRunning || t.Status == store.StatusAssigned) {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *mockStore) GetRunningTasks(_ context.Context) ([]*store.Task, error) {
	var out []*store.Task
	for _, t := range m.tasks {
		if t.Status == store.StatusRunning || t.Status == store.StatusAssigned {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *mockStore) CreateTaskEvent(_ context.Context, e *store.TaskEvent) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	m.events = append(m.events, e)
	return nil
}
func (m *mockStore) GetTaskEvents(_ context.Context, taskID uuid.UUID) ([]*store.TaskEvent, error) {
	var out []*store.TaskEvent
	for _, e := range m.events {
		if e.TaskID == taskID {
			out = append(out, e)
		}
	}
	return out, nil
}
func (m *mockStore) GetStats(_ context.Context) (*store.TaskStats, error) {
	return &store.TaskStats{}, nil
}
func (m *mockStore) Close() error { return nil }

type mockHermes struct {
	published []struct {
		subject string
		data    interface{}
	}
}

func (m *mockHermes) Publish(subject string, data interface{}) error {
	m.published = append(m.published, struct {
		subject string
		data    interface{}
	}{subject, data})
	return nil
}
func (m *mockHermes) Subscribe(_ string, _ func(string, []byte)) error { return nil }
func (m *mockHermes) Close()                                           {}

type mockWarren struct {
	states map[string]*warren.AgentState
}

func (m *mockWarren) GetAgentState(_ context.Context, id string) (*warren.AgentState, error) {
	if s, ok := m.states[id]; ok {
		return s, nil
	}
	return &warren.AgentState{Name: id, Status: "stopped"}, nil
}
func (m *mockWarren) WakeAgent(_ context.Context, _ string) error { return nil }
func (m *mockWarren) ListAgents(_ context.Context) ([]warren.AgentState, error) {
	var out []warren.AgentState
	for _, s := range m.states {
		out = append(out, *s)
	}
	return out, nil
}

type mockForge struct {
	personas []forge.Persona
}

func (m *mockForge) ListPersonas(_ context.Context) ([]forge.Persona, error) {
	return m.personas, nil
}
func (m *mockForge) GetAgentsByCapability(_ context.Context, scope string) ([]forge.Persona, error) {
	var out []forge.Persona
	for _, p := range m.personas {
		for _, c := range p.Capabilities {
			if c == scope {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
}

type mockAlexandria struct {
	devices []alexandria.Device
}

func (m *mockAlexandria) ListDevices(_ context.Context) ([]alexandria.Device, error) {
	return m.devices, nil
}
func (m *mockAlexandria) GetDevicesByOwner(_ context.Context, ownerID string) ([]alexandria.Device, error) {
	var out []alexandria.Device
	for _, d := range m.devices {
		if d.OwnerID == ownerID {
			out = append(out, d)
		}
	}
	return out, nil
}

func testConfig() *config.Config {
	return &config.Config{
		Assignment: config.AssignmentConfig{
			TickIntervalMs:        100,
			WakeTimeoutMs:         1000,
			DefaultTimeoutMs:      5000,
			MaxConcurrentPerAgent: 3,
		},
	}
}

func TestCapabilityMatch(t *testing.T) {
	p := forge.Persona{Name: "lily", Capabilities: []string{"research", "analysis"}}

	if score := CapabilityMatch(p, "research"); score != 1.0 {
		t.Errorf("expected 1.0, got %f", score)
	}
	if score := CapabilityMatch(p, "code"); score != 0 {
		t.Errorf("expected 0, got %f", score)
	}
}

func TestScoreCandidate(t *testing.T) {
	s := newMockStore()
	ctx := context.Background()
	p := forge.Persona{Name: "lily", Capabilities: []string{"research"}}
	task := &store.Task{Scope: "research", Priority: 1}

	tests := []struct {
		name   string
		status string
		policy string
		want   float64
	}{
		{"always-on ready", "ready", "always-on", 1.0 * 1.0 * 1.0 * 1.4},
		{"on-demand ready", "ready", "on-demand", 1.0 * 1.0 * 0.9 * 1.4},
		{"on-demand sleeping", "sleeping", "on-demand", 1.0 * 0.8 * 0.6 * 1.4},
		{"degraded agent", "degraded", "always-on", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &warren.AgentState{Name: "lily", Status: tt.status, Policy: tt.policy}
			got := ScoreCandidate(p, state, task, s, ctx, 3)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestPolicyMultiplier(t *testing.T) {
	tests := []struct {
		name   string
		state  *warren.AgentState
		want   float64
	}{
		{"always-on ready", &warren.AgentState{Policy: "always-on", Status: "ready"}, 1.0},
		{"on-demand ready", &warren.AgentState{Policy: "on-demand", Status: "ready"}, 0.9},
		{"on-demand sleeping", &warren.AgentState{Policy: "on-demand", Status: "sleeping"}, 0.6},
		{"on-demand busy", &warren.AgentState{Policy: "on-demand", Status: "busy"}, 0.9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PolicyMultiplier(tt.state)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestBrokerAssignment(t *testing.T) {
	ms := newMockStore()
	mh := &mockHermes{}
	mw := &mockWarren{states: map[string]*warren.AgentState{
		"lily": {Name: "lily", Status: "ready", Policy: "always-on"},
		"nova": {Name: "nova", Status: "sleeping", Policy: "on-demand"},
	}}
	mf := &mockForge{personas: []forge.Persona{
		{Name: "lily", Capabilities: []string{"research", "analysis"}},
		{Name: "nova", Capabilities: []string{"research", "code"}},
	}}

	cfg := testConfig()
	b := New(ms, mh, mw, mf, nil, cfg, discardLogger())

	ctx := context.Background()
	task := &store.Task{
		Requester: "main",
		Title:     "test task",
		Scope:     "research",
		Priority:  3,
		Status:    store.StatusPending,
		TimeoutMs: 5000,
	}
	_ = ms.CreateTask(ctx, task)

	b.processPendingTasks(ctx)

	updated := ms.tasks[task.ID]
	if updated.Status != store.StatusAssigned {
		t.Errorf("expected assigned, got %s", updated.Status)
	}
	if updated.Assignee != "lily" {
		t.Errorf("expected lily (ready+always-on), got %s", updated.Assignee)
	}
}

func TestOwnerScopedFiltering(t *testing.T) {
	ms := newMockStore()
	mh := &mockHermes{}
	ownerID := uuid.New().String()
	mw := &mockWarren{states: map[string]*warren.AgentState{
		"lily": {Name: "lily", Status: "ready", Policy: "always-on"},
		"nova": {Name: "nova", Status: "ready", Policy: "always-on"},
	}}
	mf := &mockForge{personas: []forge.Persona{
		{Name: "lily", Capabilities: []string{"research"}},
		{Name: "nova", Capabilities: []string{"research"}},
	}}
	ma := &mockAlexandria{devices: []alexandria.Device{
		{ID: "d1", Name: "nova", OwnerID: ownerID},
	}}

	cfg := testConfig()
	b := New(ms, mh, mw, mf, ma, cfg, discardLogger())

	ctx := context.Background()
	task := &store.Task{
		Requester: "main",
		Owner:     ownerID,
		Submitter: "main",
		Title:     "owner-scoped task",
		Scope:     "research",
		Priority:  3,
		Status:    store.StatusPending,
		TimeoutMs: 5000,
	}
	_ = ms.CreateTask(ctx, task)

	b.processPendingTasks(ctx)

	updated := ms.tasks[task.ID]
	if updated.Status != store.StatusAssigned {
		t.Errorf("expected assigned, got %s", updated.Status)
	}
	// Should be nova (only agent owned by this owner)
	if updated.Assignee != "nova" {
		t.Errorf("expected nova (owner-scoped), got %s", updated.Assignee)
	}
}

func TestOwnerScopedUnmatched(t *testing.T) {
	ms := newMockStore()
	mh := &mockHermes{}
	ownerID := uuid.New().String()
	mw := &mockWarren{states: map[string]*warren.AgentState{
		"lily": {Name: "lily", Status: "ready", Policy: "always-on"},
	}}
	mf := &mockForge{personas: []forge.Persona{
		{Name: "lily", Capabilities: []string{"research"}},
	}}
	// No devices owned by ownerID
	ma := &mockAlexandria{devices: []alexandria.Device{
		{ID: "d1", Name: "lily", OwnerID: "different-owner"},
	}}

	cfg := testConfig()
	b := New(ms, mh, mw, mf, ma, cfg, discardLogger())

	ctx := context.Background()
	task := &store.Task{
		Requester: "main",
		Owner:     ownerID,
		Title:     "unmatched task",
		Scope:     "research",
		Priority:  3,
		Status:    store.StatusPending,
		TimeoutMs: 5000,
	}
	_ = ms.CreateTask(ctx, task)

	b.processPendingTasks(ctx)

	updated := ms.tasks[task.ID]
	if updated.Status != store.StatusPending {
		t.Errorf("expected pending (unmatched), got %s", updated.Status)
	}

	// Should have published unmatched event
	found := false
	for _, p := range mh.published {
		if p.subject == "swarm.task."+task.ID.String()+".unmatched" {
			found = true
		}
	}
	if !found {
		t.Error("expected unmatched event to be published")
	}
}

func TestBrokerDrain(t *testing.T) {
	b := New(nil, nil, nil, nil, nil, testConfig(), discardLogger())
	b.DrainAgent("lily")
	if !b.IsDrained("lily") {
		t.Error("expected lily to be drained")
	}
	b.UndrainAgent("lily")
	if b.IsDrained("lily") {
		t.Error("expected lily to be undrained")
	}
}

func TestTimeoutRetry(t *testing.T) {
	ms := newMockStore()
	mh := &mockHermes{}
	cfg := testConfig()
	b := New(ms, mh, nil, nil, nil, cfg, discardLogger())

	ctx := context.Background()
	past := time.Now().Add(-10 * time.Second)
	task := &store.Task{
		Requester:  "main",
		Title:      "timeout test",
		Scope:      "research",
		Priority:   3,
		Status:     store.StatusRunning,
		Assignee:   "lily",
		TimeoutMs:  1000,
		MaxRetries: 2,
		RetryCount: 0,
		AssignedAt: &past,
		StartedAt:  &past,
	}
	_ = ms.CreateTask(ctx, task)

	b.checkTimeouts(ctx)

	updated := ms.tasks[task.ID]
	if updated.Status != store.StatusPending {
		t.Errorf("expected pending (retry), got %s", updated.Status)
	}
	if updated.RetryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", updated.RetryCount)
	}
}

func TestTimeoutExhausted(t *testing.T) {
	ms := newMockStore()
	mh := &mockHermes{}
	cfg := testConfig()
	b := New(ms, mh, nil, nil, nil, cfg, discardLogger())

	ctx := context.Background()
	past := time.Now().Add(-10 * time.Second)
	task := &store.Task{
		Requester:  "main",
		Title:      "timeout exhausted",
		Scope:      "research",
		Priority:   3,
		Status:     store.StatusRunning,
		Assignee:   "lily",
		TimeoutMs:  1000,
		MaxRetries: 1,
		RetryCount: 1,
		AssignedAt: &past,
		StartedAt:  &past,
	}
	_ = ms.CreateTask(ctx, task)

	b.checkTimeouts(ctx)

	updated := ms.tasks[task.ID]
	if updated.Status != store.StatusTimeout {
		t.Errorf("expected timeout, got %s", updated.Status)
	}
}

func TestHandleAgentStopped(t *testing.T) {
	ms := newMockStore()
	cfg := testConfig()
	b := New(ms, &mockHermes{}, nil, nil, nil, cfg, discardLogger())

	ctx := context.Background()
	now := time.Now()
	task := &store.Task{
		Requester:  "main",
		Title:      "running task",
		Scope:      "code",
		Priority:   3,
		Status:     store.StatusRunning,
		Assignee:   "lily",
		AssignedAt: &now,
		StartedAt:  &now,
	}
	_ = ms.CreateTask(ctx, task)

	b.HandleAgentStopped(ctx, "lily")

	updated := ms.tasks[task.ID]
	if updated.Status != store.StatusPending {
		t.Errorf("expected pending, got %s", updated.Status)
	}
	if updated.Assignee != "" {
		t.Errorf("expected empty assignee, got %s", updated.Assignee)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
