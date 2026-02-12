package broker

import (
	"context"
	"strings"

	"github.com/DarlingtonDeveloper/Dispatch/internal/forge"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
	"github.com/DarlingtonDeveloper/Dispatch/internal/warren"
)

// CapabilityMatch returns a score 0-1 for how well a persona matches a task scope.
func CapabilityMatch(persona forge.Persona, scope string) float64 {
	for _, cap := range persona.Capabilities {
		if strings.EqualFold(cap, scope) {
			return 1.0
		}
	}
	return 0
}

// ScoreCandidate computes the assignment score for a candidate.
func ScoreCandidate(persona forge.Persona, state *warren.AgentState, task *store.Task, s store.Store, ctx context.Context, maxConcurrent int) float64 {
	capScore := CapabilityMatch(persona, task.Scope)
	if capScore == 0 {
		return 0
	}

	var availMultiplier float64
	switch state.Status {
	case "ready":
		availMultiplier = 1.0
	case "sleeping":
		availMultiplier = 0.8
	case "busy":
		// Check concurrent tasks
		running, err := s.GetRunningTasksForAgent(ctx, persona.Name)
		if err != nil || len(running) >= maxConcurrent {
			return 0
		}
		availMultiplier = 0.5
	default: // degraded, stopped, etc.
		return 0
	}

	// Priority weight: higher priority (lower number) gets a boost
	priorityWeight := 1.0 + float64(5-task.Priority)*0.1

	return capScore * availMultiplier * priorityWeight
}
