package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DarlingtonDeveloper/Dispatch/internal/broker"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
	"github.com/DarlingtonDeveloper/Dispatch/internal/warren"
	"github.com/DarlingtonDeveloper/Dispatch/internal/forge"
)

type AdminHandler struct {
	store  store.Store
	warren warren.Client
	forge  forge.Client
	broker *broker.Broker
}

func NewAdminHandler(s store.Store, w warren.Client, f forge.Client, b *broker.Broker) *AdminHandler {
	return &AdminHandler{store: s, warren: w, forge: f, broker: b}
}

func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type AgentInfo struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities,omitempty"`
	ActiveTasks  int      `json:"active_tasks"`
	Drained      bool     `json:"drained"`
}

func (h *AdminHandler) Agents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.warren.ListAgents(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	personas, _ := h.forge.ListPersonas(r.Context())
	capMap := make(map[string][]string)
	for _, p := range personas {
		capMap[p.Name] = p.Capabilities
	}

	var infos []AgentInfo
	for _, a := range agents {
		running, _ := h.store.GetActiveTasksForAgent(r.Context(), a.Name)
		infos = append(infos, AgentInfo{
			Name:         a.Name,
			Status:       a.Status,
			Capabilities: capMap[a.Name],
			ActiveTasks:  len(running),
			Drained:      h.broker.IsDrained(a.Name),
		})
	}

	writeJSON(w, http.StatusOK, infos)
}

func (h *AdminHandler) Drain(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	h.broker.DrainAgent(agentID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "drained", "agent": agentID})
}
