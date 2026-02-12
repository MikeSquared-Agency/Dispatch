package hermes

import "time"

type TaskRequestEvent struct {
	Requester   string                 `json:"requester"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Scope       string                 `json:"scope"`
	Priority    int                    `json:"priority,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	TimeoutMs   int                    `json:"timeout_ms,omitempty"`
	MaxRetries  int                    `json:"max_retries,omitempty"`
}

type TaskAssignedEvent struct {
	TaskID   string `json:"task_id"`
	Assignee string `json:"assignee"`
	Scope    string `json:"scope"`
}

type TaskCompletedEvent struct {
	TaskID string                 `json:"task_id"`
	Result map[string]interface{} `json:"result,omitempty"`
}

type TaskFailedEvent struct {
	TaskID string `json:"task_id"`
	Error  string `json:"error"`
}

type TaskTimeoutEvent struct {
	TaskID     string `json:"task_id"`
	RetryCount int    `json:"retry_count"`
	MaxRetries int    `json:"max_retries"`
}

type StatsEvent struct {
	Pending   int     `json:"pending"`
	Running   int     `json:"running"`
	Completed int     `json:"completed"`
	Failed    int     `json:"failed"`
	AvgMs     float64 `json:"avg_completion_ms"`
	Timestamp time.Time `json:"timestamp"`
}
