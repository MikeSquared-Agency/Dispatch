package hermes

import "time"

type TaskRequestEvent struct {
	Owner                string                 `json:"owner"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description,omitempty"`
	RequiredCapabilities []string               `json:"required_capabilities,omitempty"`
	Priority             int                    `json:"priority,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	TimeoutSeconds       int                    `json:"timeout_seconds,omitempty"`
	MaxRetries           int                    `json:"max_retries,omitempty"`
	Source               string                 `json:"source,omitempty"`
}

type TaskAssignedEvent struct {
	TaskID        string `json:"task_id"`
	AssignedAgent string `json:"assigned_agent"`
}

type TaskCompletedEvent struct {
	TaskID string                 `json:"task_id"`
	Result map[string]interface{} `json:"result,omitempty"`
}

type TaskFailedEvent struct {
	TaskID        string `json:"task_id"`
	Error         string `json:"error"`
	RetryEligible bool   `json:"retry_eligible"`
}

type TaskTimeoutEvent struct {
	TaskID     string `json:"task_id"`
	RetryCount int    `json:"retry_count"`
	MaxRetries int    `json:"max_retries"`
	TimedOutIn string `json:"timed_out_in_state"`
}

type StatsEvent struct {
	Pending    int       `json:"pending"`
	InProgress int       `json:"in_progress"`
	Completed  int       `json:"completed"`
	Failed     int       `json:"failed"`
	AvgMs      float64   `json:"avg_completion_ms"`
	Timestamp  time.Time `json:"timestamp"`
}
