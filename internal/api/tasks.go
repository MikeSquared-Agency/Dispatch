package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/DarlingtonDeveloper/Dispatch/internal/hermes"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
)

type TasksHandler struct {
	store  store.Store
	hermes hermes.Client
}

func NewTasksHandler(s store.Store, h hermes.Client) *TasksHandler {
	return &TasksHandler{store: s, hermes: h}
}

type CreateTaskRequest struct {
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Scope       string                 `json:"scope"`
	Owner       string                 `json:"owner,omitempty"`
	Priority    int                    `json:"priority,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	TimeoutMs   int                    `json:"timeout_ms,omitempty"`
	MaxRetries  int                    `json:"max_retries,omitempty"`
	ParentID    string                 `json:"parent_id,omitempty"`
}

func (h *TasksHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Title == "" || req.Scope == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title and scope required"})
		return
	}

	submitter := r.Header.Get("X-Agent-ID")

	task := &store.Task{
		Requester:   submitter,
		Owner:       req.Owner,
		Submitter:   submitter,
		Title:       req.Title,
		Description: req.Description,
		Scope:       req.Scope,
		Priority:    req.Priority,
		Status:      store.StatusPending,
		Context:     req.Context,
		TimeoutMs:   req.TimeoutMs,
		MaxRetries:  req.MaxRetries,
	}
	if task.Priority == 0 {
		task.Priority = 3
	}
	if task.TimeoutMs == 0 {
		task.TimeoutMs = 300000
	}
	if task.MaxRetries == 0 {
		task.MaxRetries = 1
	}
	if req.ParentID != "" {
		pid, err := uuid.Parse(req.ParentID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid parent_id"})
			return
		}
		task.ParentID = &pid
	}

	if err := h.store.CreateTask(r.Context(), task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, task)
}

func (h *TasksHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := store.TaskFilter{
		Requester: r.URL.Query().Get("requester"),
		Assignee:  r.URL.Query().Get("assignee"),
		Scope:     r.URL.Query().Get("scope"),
		Owner:     r.URL.Query().Get("owner"),
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := store.TaskStatus(s)
		filter.Status = &status
	}

	tasks, err := h.store.ListTasks(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tasks == nil {
		tasks = []*store.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *TasksHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task id"})
		return
	}

	task, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *TasksHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task id"})
		return
	}

	task, err := h.store.GetTask(r.Context(), id)
	if err != nil || task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if s, ok := patch["status"].(string); ok && s == "cancelled" {
		task.Status = store.StatusCancelled
		now := time.Now()
		task.CompletedAt = &now
	}
	if ctx, ok := patch["context"].(map[string]interface{}); ok {
		task.Context = ctx
	}

	if err := h.store.UpdateTask(r.Context(), task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *TasksHandler) Complete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task id"})
		return
	}

	task, err := h.store.GetTask(r.Context(), id)
	if err != nil || task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	var body struct {
		Result map[string]interface{} `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	now := time.Now()
	task.Status = store.StatusCompleted
	task.Result = body.Result
	task.CompletedAt = &now

	if err := h.store.UpdateTask(r.Context(), task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_ = h.store.CreateTaskEvent(r.Context(), &store.TaskEvent{
		TaskID:  task.ID,
		Event:   "completed",
		AgentID: r.Header.Get("X-Agent-ID"),
	})

	if h.hermes != nil {
		_ = h.hermes.Publish(hermes.SubjectTaskCompleted(task.ID.String()), hermes.TaskCompletedEvent{
			TaskID: task.ID.String(),
			Result: body.Result,
		})
	}

	writeJSON(w, http.StatusOK, task)
}

func (h *TasksHandler) Fail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task id"})
		return
	}

	task, err := h.store.GetTask(r.Context(), id)
	if err != nil || task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	now := time.Now()
	task.Status = store.StatusFailed
	task.Error = body.Error
	task.CompletedAt = &now

	if err := h.store.UpdateTask(r.Context(), task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_ = h.store.CreateTaskEvent(r.Context(), &store.TaskEvent{
		TaskID:  task.ID,
		Event:   "failed",
		AgentID: r.Header.Get("X-Agent-ID"),
	})

	if h.hermes != nil {
		_ = h.hermes.Publish(hermes.SubjectTaskFailed(task.ID.String()), hermes.TaskFailedEvent{
			TaskID: task.ID.String(),
			Error:  body.Error,
		})
	}

	writeJSON(w, http.StatusOK, task)
}

func (h *TasksHandler) Progress(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task id"})
		return
	}

	task, err := h.store.GetTask(r.Context(), id)
	if err != nil || task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	if task.Status == store.StatusAssigned {
		now := time.Now()
		task.Status = store.StatusRunning
		task.StartedAt = &now
		if err := h.store.UpdateTask(r.Context(), task); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)

	_ = h.store.CreateTaskEvent(r.Context(), &store.TaskEvent{
		TaskID:  task.ID,
		Event:   "progress",
		AgentID: r.Header.Get("X-Agent-ID"),
		Payload: body,
	})

	if h.hermes != nil {
		_ = h.hermes.Publish(hermes.SubjectTaskProgress(task.ID.String()), body)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
