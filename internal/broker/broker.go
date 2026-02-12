package broker

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/DarlingtonDeveloper/Dispatch/internal/config"
	"github.com/DarlingtonDeveloper/Dispatch/internal/forge"
	"github.com/DarlingtonDeveloper/Dispatch/internal/hermes"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
	"github.com/DarlingtonDeveloper/Dispatch/internal/warren"
)

type Broker struct {
	store   store.Store
	hermes  hermes.Client
	warren  warren.Client
	forge   forge.Client
	cfg     *config.Config
	logger  *slog.Logger

	drainedMu sync.RWMutex
	drained   map[string]bool // agents that are drained

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func New(s store.Store, h hermes.Client, w warren.Client, f forge.Client, cfg *config.Config, logger *slog.Logger) *Broker {
	return &Broker{
		store:   s,
		hermes:  h,
		warren:  w,
		forge:   f,
		cfg:     cfg,
		logger:  logger,
		drained: make(map[string]bool),
		stopCh:  make(chan struct{}),
	}
}

func (b *Broker) Start(ctx context.Context) {
	b.wg.Add(2)
	go b.assignmentLoop(ctx)
	go b.timeoutLoop(ctx)
}

func (b *Broker) Stop() {
	close(b.stopCh)
	b.wg.Wait()
}

func (b *Broker) DrainAgent(agentID string) {
	b.drainedMu.Lock()
	b.drained[agentID] = true
	b.drainedMu.Unlock()
}

func (b *Broker) UndrainAgent(agentID string) {
	b.drainedMu.Lock()
	delete(b.drained, agentID)
	b.drainedMu.Unlock()
}

func (b *Broker) IsDrained(agentID string) bool {
	b.drainedMu.RLock()
	defer b.drainedMu.RUnlock()
	return b.drained[agentID]
}

func (b *Broker) assignmentLoop(ctx context.Context) {
	defer b.wg.Done()
	ticker := time.NewTicker(b.cfg.TickInterval())
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.processPendingTasks(ctx)
		}
	}
}

func (b *Broker) processPendingTasks(ctx context.Context) {
	tasks, err := b.store.GetPendingTasks(ctx)
	if err != nil {
		b.logger.Error("failed to get pending tasks", "error", err)
		return
	}

	for _, task := range tasks {
		if err := b.assignTask(ctx, task); err != nil {
			b.logger.Warn("failed to assign task", "task_id", task.ID, "error", err)
		}
	}
}

func (b *Broker) assignTask(ctx context.Context, task *store.Task) error {
	candidates, err := b.forge.GetAgentsByCapability(ctx, task.Scope)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		_ = b.hermes.Publish(hermes.SubjectTaskUnmatched(task.ID.String()), map[string]string{
			"task_id": task.ID.String(),
			"scope":   task.Scope,
		})
		return nil
	}

	type scored struct {
		persona forge.Persona
		score   float64
	}
	var scoredCandidates []scored

	for _, c := range candidates {
		if b.IsDrained(c.Name) {
			continue
		}
		state, err := b.warren.GetAgentState(ctx, c.Name)
		if err != nil {
			b.logger.Warn("failed to get agent state", "agent", c.Name, "error", err)
			continue
		}

		s := ScoreCandidate(c, state, task, b.store, ctx, b.cfg.Assignment.MaxConcurrentPerAgent)
		if s > 0 {
			scoredCandidates = append(scoredCandidates, scored{persona: c, score: s})
		}
	}

	if len(scoredCandidates) == 0 {
		return nil
	}

	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	winner := scoredCandidates[0]

	// Wake if sleeping
	state, _ := b.warren.GetAgentState(ctx, winner.persona.Name)
	if state != nil && state.Status == "sleeping" {
		if err := b.warren.WakeAgent(ctx, winner.persona.Name); err != nil {
			b.logger.Warn("failed to wake agent", "agent", winner.persona.Name, "error", err)
			return nil
		}
		// Brief wait for agent to start (simplified; production would listen for started event)
		time.Sleep(2 * time.Second)
	}

	now := time.Now()
	task.Status = store.StatusAssigned
	task.Assignee = winner.persona.Name
	task.AssignedAt = &now

	if err := b.store.UpdateTask(ctx, task); err != nil {
		return err
	}

	_ = b.store.CreateTaskEvent(ctx, &store.TaskEvent{
		TaskID:  task.ID,
		Event:   "assigned",
		AgentID: winner.persona.Name,
	})

	_ = b.hermes.Publish(hermes.SubjectTaskAssigned(task.ID.String()), hermes.TaskAssignedEvent{
		TaskID:   task.ID.String(),
		Assignee: winner.persona.Name,
		Scope:    task.Scope,
	})

	b.logger.Info("task assigned", "task_id", task.ID, "assignee", winner.persona.Name, "score", winner.score)
	return nil
}

func (b *Broker) HandleAgentStopped(ctx context.Context, agentID string) {
	tasks, err := b.store.GetRunningTasksForAgent(ctx, agentID)
	if err != nil {
		b.logger.Error("failed to get tasks for stopped agent", "agent", agentID, "error", err)
		return
	}
	for _, task := range tasks {
		task.Status = store.StatusPending
		task.Assignee = ""
		task.AssignedAt = nil
		task.StartedAt = nil
		if err := b.store.UpdateTask(ctx, task); err != nil {
			b.logger.Error("failed to reset task", "task_id", task.ID, "error", err)
		}
	}
}
