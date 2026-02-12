package broker

import (
	"context"
	"strings"

	"github.com/DarlingtonDeveloper/Dispatch/internal/forge"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
	"github.com/DarlingtonDeveloper/Dispatch/internal/warren"
)

// CapabilityMatch returns 1.0 if the persona satisfies ALL required capabilities, 0 otherwise.
func CapabilityMatch(persona forge.Persona, requiredCapabilities []string) float64 {
	if len(requiredCapabilities) == 0 {
		return 1.0
	}
	for _, req := range requiredCapabilities {
		found := false
		for _, cap := range persona.Capabilities {
			if strings.EqualFold(cap, req) {
				found = true
				break
			}
		}
		if !found {
			return 0
		}
	}
	return 1.0
}

// PolicyMultiplier returns a scoring multiplier based on agent policy and current state.
//   - always-on + ready → 1.0
//   - on-demand + already awake (ready/busy) → 0.9
//   - on-demand + sleeping → 0.6
func PolicyMultiplier(state *warren.AgentState) float64 {
	switch state.Policy {
	case "always-on":
		if state.Status == "ready" {
			return 1.0
		}
		// always-on but busy/sleeping still gets base availability
		return 1.0
	case "on-demand":
		switch state.Status {
		case "ready", "busy":
			return 0.9
		case "sleeping":
			return 0.6
		}
		return 0.6
	default:
		// Unknown policy, treat like on-demand
		switch state.Status {
		case "ready":
			return 1.0
		case "sleeping":
			return 0.6
		default:
			return 0.9
		}
	}
}

// ScoreCandidate computes the assignment score for a candidate.
func ScoreCandidate(persona forge.Persona, state *warren.AgentState, task *store.Task, s store.Store, ctx context.Context, maxConcurrent int) float64 {
	capScore := CapabilityMatch(persona, task.RequiredCapabilities)
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
		active, err := s.GetActiveTasksForAgent(ctx, persona.Slug)
		if err != nil || len(active) >= maxConcurrent {
			return 0
		}
		availMultiplier = 0.5
	default: // degraded, stopped, etc.
		return 0
	}

	// Apply policy-based multiplier
	policyMult := PolicyMultiplier(state)

	// Priority weight: higher priority (0-10) gets a boost
	priorityWeight := 1.0 + float64(task.Priority)*0.05

	return capScore * availMultiplier * policyMult * priorityWeight
}
