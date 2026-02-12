package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusAssigned  TaskStatus = "assigned"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusCancelled TaskStatus = "cancelled"
	StatusTimeout   TaskStatus = "timeout"
)

type Task struct {
	ID          uuid.UUID              `json:"id"`
	Requester   string                 `json:"requester"`
	Owner       string                 `json:"owner,omitempty"`
	Submitter   string                 `json:"submitter,omitempty"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Scope       string                 `json:"scope"`
	Priority    int                    `json:"priority"`
	Status      TaskStatus             `json:"status"`
	Assignee    string                 `json:"assignee,omitempty"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	TimeoutMs   int                    `json:"timeout_ms"`
	MaxRetries  int                    `json:"max_retries"`
	RetryCount  int                    `json:"retry_count"`
	ParentID    *uuid.UUID             `json:"parent_id,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	AssignedAt  *time.Time             `json:"assigned_at,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

type TaskFilter struct {
	Status    *TaskStatus
	Requester string
	Assignee  string
	Scope     string
	Owner     string
	Limit     int
	Offset    int
}

type TaskEvent struct {
	ID        uuid.UUID              `json:"id"`
	TaskID    uuid.UUID              `json:"task_id"`
	Event     string                 `json:"event"`
	AgentID   string                 `json:"agent_id,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type TaskStats struct {
	TotalPending   int     `json:"total_pending"`
	TotalRunning   int     `json:"total_running"`
	TotalCompleted int     `json:"total_completed"`
	TotalFailed    int     `json:"total_failed"`
	AvgCompletionMs float64 `json:"avg_completion_ms"`
}

type Store interface {
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)
	UpdateTask(ctx context.Context, task *Task) error
	
	GetPendingTasks(ctx context.Context) ([]*Task, error)
	GetRunningTasksForAgent(ctx context.Context, agentID string) ([]*Task, error)
	GetRunningTasks(ctx context.Context) ([]*Task, error)
	
	CreateTaskEvent(ctx context.Context, event *TaskEvent) error
	GetTaskEvents(ctx context.Context, taskID uuid.UUID) ([]*TaskEvent, error)
	
	GetStats(ctx context.Context) (*TaskStats, error)
	
	Close() error
}
