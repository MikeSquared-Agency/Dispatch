package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

const taskColumns = `task_id, title, description, owner, required_capabilities,
	status, assigned_agent,
	created_at, assigned_at, started_at, completed_at, updated_at,
	result, error,
	retry_count, max_retries, retry_eligible,
	timeout_seconds,
	priority, source, parent_task_id, metadata`

func (s *PostgresStore) CreateTask(ctx context.Context, task *Task) error {
	resultJSON, _ := json.Marshal(task.Result)
	metadataJSON, _ := json.Marshal(task.Metadata)

	return s.pool.QueryRow(ctx, `
		INSERT INTO swarm_tasks (title, description, owner, required_capabilities,
			status, timeout_seconds, max_retries, retry_eligible,
			priority, source, parent_task_id, result, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING task_id, created_at, updated_at`,
		task.Title, task.Description, task.Owner, task.RequiredCapabilities,
		task.Status, task.TimeoutSeconds, task.MaxRetries, task.RetryEligible,
		task.Priority, task.Source, task.ParentTaskID, resultJSON, metadataJSON,
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
}

func (s *PostgresStore) GetTask(ctx context.Context, id uuid.UUID) (*Task, error) {
	t := &Task{}
	var resultJSON, metadataJSON []byte
	var assignedAgent, taskError sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT `+taskColumns+`
		FROM swarm_tasks WHERE task_id = $1`, id,
	).Scan(
		&t.ID, &t.Title, &t.Description, &t.Owner, &t.RequiredCapabilities,
		&t.Status, &assignedAgent,
		&t.CreatedAt, &t.AssignedAt, &t.StartedAt, &t.CompletedAt, &t.UpdatedAt,
		&resultJSON, &taskError,
		&t.RetryCount, &t.MaxRetries, &t.RetryEligible,
		&t.TimeoutSeconds,
		&t.Priority, &t.Source, &t.ParentTaskID, &metadataJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if assignedAgent.Valid {
		t.AssignedAgent = assignedAgent.String
	}
	if taskError.Valid {
		t.Error = taskError.String
	}
	if resultJSON != nil {
		_ = json.Unmarshal(resultJSON, &t.Result)
	}
	if metadataJSON != nil {
		_ = json.Unmarshal(metadataJSON, &t.Metadata)
	}
	return t, nil
}

func (s *PostgresStore) ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error) {
	query := `SELECT ` + taskColumns + ` FROM swarm_tasks WHERE 1=1`
	args := []interface{}{}
	n := 0

	if filter.Status != nil {
		n++
		query += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, string(*filter.Status))
	}
	if filter.Agent != "" {
		n++
		query += fmt.Sprintf(" AND assigned_agent = $%d", n)
		args = append(args, filter.Agent)
	}
	if filter.Owner != "" {
		n++
		query += fmt.Sprintf(" AND owner = $%d", n)
		args = append(args, filter.Owner)
	}
	if filter.Source != "" {
		n++
		query += fmt.Sprintf(" AND source = $%d", n)
		args = append(args, filter.Source)
	}

	query += " ORDER BY priority DESC, created_at ASC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	n++
	query += fmt.Sprintf(" LIMIT $%d", n)
	args = append(args, limit)

	if filter.Offset > 0 {
		n++
		query += fmt.Sprintf(" OFFSET $%d", n)
		args = append(args, filter.Offset)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (s *PostgresStore) GetPendingTasks(ctx context.Context) ([]*Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+`
		FROM swarm_tasks WHERE status = 'pending'
		ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) GetActiveTasksForAgent(ctx context.Context, agentID string) ([]*Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+`
		FROM swarm_tasks WHERE assigned_agent = $1 AND status IN ('assigned', 'in_progress')`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) GetActiveTasks(ctx context.Context) ([]*Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+`
		FROM swarm_tasks WHERE status IN ('assigned', 'in_progress')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) UpdateTask(ctx context.Context, task *Task) error {
	resultJSON, _ := json.Marshal(task.Result)
	metadataJSON, _ := json.Marshal(task.Metadata)

	_, err := s.pool.Exec(ctx, `
		UPDATE swarm_tasks SET
			title = $2, description = $3, owner = $4, required_capabilities = $5,
			status = $6, assigned_agent = $7,
			assigned_at = $8, started_at = $9, completed_at = $10,
			result = $11, error = $12,
			retry_count = $13, max_retries = $14, retry_eligible = $15,
			timeout_seconds = $16,
			priority = $17, source = $18, parent_task_id = $19, metadata = $20
		WHERE task_id = $1`,
		task.ID, task.Title, task.Description, task.Owner, task.RequiredCapabilities,
		task.Status, task.AssignedAgent,
		task.AssignedAt, task.StartedAt, task.CompletedAt,
		resultJSON, task.Error,
		task.RetryCount, task.MaxRetries, task.RetryEligible,
		task.TimeoutSeconds,
		task.Priority, task.Source, task.ParentTaskID, metadataJSON,
	)
	return err
}

func (s *PostgresStore) CreateTaskEvent(ctx context.Context, event *TaskEvent) error {
	payloadJSON, _ := json.Marshal(event.Payload)
	return s.pool.QueryRow(ctx, `
		INSERT INTO swarm_task_events (task_id, event, agent_id, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		event.TaskID, event.Event, event.AgentID, payloadJSON,
	).Scan(&event.ID, &event.CreatedAt)
}

func (s *PostgresStore) GetTaskEvents(ctx context.Context, taskID uuid.UUID) ([]*TaskEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, task_id, event, agent_id, payload, created_at
		FROM swarm_task_events WHERE task_id = $1
		ORDER BY created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*TaskEvent
	for rows.Next() {
		e := &TaskEvent{}
		var payloadJSON []byte
		if err := rows.Scan(&e.ID, &e.TaskID, &e.Event, &e.AgentID, &payloadJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		if payloadJSON != nil {
			_ = json.Unmarshal(payloadJSON, &e.Payload)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *PostgresStore) GetStats(ctx context.Context) (*TaskStats, error) {
	stats := &TaskStats{}
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('assigned','in_progress') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(EXTRACT(EPOCH FROM (completed_at - assigned_at)) * 1000) FILTER (WHERE status = 'completed' AND completed_at IS NOT NULL AND assigned_at IS NOT NULL), 0)
		FROM swarm_tasks`,
	).Scan(&stats.TotalPending, &stats.TotalInProgress, &stats.TotalCompleted, &stats.TotalFailed, &stats.AvgCompletionMs)
	return stats, err
}

func scanTasks(rows pgx.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		var resultJSON, metadataJSON []byte
		var assignedAgent, taskError sql.NullString
		if err := rows.Scan(
			&t.ID, &t.Title, &t.Description, &t.Owner, &t.RequiredCapabilities,
			&t.Status, &assignedAgent,
			&t.CreatedAt, &t.AssignedAt, &t.StartedAt, &t.CompletedAt, &t.UpdatedAt,
			&resultJSON, &taskError,
			&t.RetryCount, &t.MaxRetries, &t.RetryEligible,
			&t.TimeoutSeconds,
			&t.Priority, &t.Source, &t.ParentTaskID, &metadataJSON,
		); err != nil {
			return nil, err
		}
		if assignedAgent.Valid {
			t.AssignedAgent = assignedAgent.String
		}
		if taskError.Valid {
			t.Error = taskError.String
		}
		if resultJSON != nil {
			_ = json.Unmarshal(resultJSON, &t.Result)
		}
		if metadataJSON != nil {
			_ = json.Unmarshal(metadataJSON, &t.Metadata)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
