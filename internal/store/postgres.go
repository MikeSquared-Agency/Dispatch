package store

import (
	"context"
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

const taskColumns = `id, requester, owner, submitter, title, description, scope, priority, status, assignee,
	result, error, context, timeout_ms, max_retries, retry_count, parent_id,
	created_at, assigned_at, started_at, completed_at`

func (s *PostgresStore) CreateTask(ctx context.Context, task *Task) error {
	resultJSON, _ := json.Marshal(task.Result)
	contextJSON, _ := json.Marshal(task.Context)

	var ownerUUID *uuid.UUID
	if task.Owner != "" {
		if parsed, err := uuid.Parse(task.Owner); err == nil {
			ownerUUID = &parsed
		}
	}

	return s.pool.QueryRow(ctx, `
		INSERT INTO dispatch_tasks (requester, owner, submitter, title, description, scope, priority, status, context, timeout_ms, max_retries, parent_id, result)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at`,
		task.Requester, ownerUUID, task.Submitter, task.Title, task.Description, task.Scope, task.Priority,
		task.Status, contextJSON, task.TimeoutMs, task.MaxRetries, task.ParentID, resultJSON,
	).Scan(&task.ID, &task.CreatedAt)
}

func (s *PostgresStore) GetTask(ctx context.Context, id uuid.UUID) (*Task, error) {
	t := &Task{}
	var resultJSON, contextJSON []byte
	var ownerUUID *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT `+taskColumns+`
		FROM dispatch_tasks WHERE id = $1`, id,
	).Scan(
		&t.ID, &t.Requester, &ownerUUID, &t.Submitter, &t.Title, &t.Description, &t.Scope, &t.Priority,
		&t.Status, &t.Assignee, &resultJSON, &t.Error, &contextJSON,
		&t.TimeoutMs, &t.MaxRetries, &t.RetryCount, &t.ParentID,
		&t.CreatedAt, &t.AssignedAt, &t.StartedAt, &t.CompletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ownerUUID != nil {
		t.Owner = ownerUUID.String()
	}
	if resultJSON != nil {
		_ = json.Unmarshal(resultJSON, &t.Result)
	}
	if contextJSON != nil {
		_ = json.Unmarshal(contextJSON, &t.Context)
	}
	return t, nil
}

func (s *PostgresStore) ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error) {
	query := `SELECT ` + taskColumns + ` FROM dispatch_tasks WHERE 1=1`
	args := []interface{}{}
	n := 0

	if filter.Status != nil {
		n++
		query += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, string(*filter.Status))
	}
	if filter.Requester != "" {
		n++
		query += fmt.Sprintf(" AND requester = $%d", n)
		args = append(args, filter.Requester)
	}
	if filter.Assignee != "" {
		n++
		query += fmt.Sprintf(" AND assignee = $%d", n)
		args = append(args, filter.Assignee)
	}
	if filter.Scope != "" {
		n++
		query += fmt.Sprintf(" AND scope = $%d", n)
		args = append(args, filter.Scope)
	}
	if filter.Owner != "" {
		n++
		query += fmt.Sprintf(" AND owner = $%d", n)
		args = append(args, filter.Owner)
	}

	query += " ORDER BY priority ASC, created_at ASC"

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
		FROM dispatch_tasks WHERE status = 'pending'
		ORDER BY priority ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) GetRunningTasksForAgent(ctx context.Context, agentID string) ([]*Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+`
		FROM dispatch_tasks WHERE assignee = $1 AND status IN ('assigned', 'running')`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) GetRunningTasks(ctx context.Context) ([]*Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+`
		FROM dispatch_tasks WHERE status IN ('assigned', 'running')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) UpdateTask(ctx context.Context, task *Task) error {
	resultJSON, _ := json.Marshal(task.Result)
	contextJSON, _ := json.Marshal(task.Context)

	var ownerUUID *uuid.UUID
	if task.Owner != "" {
		if parsed, err := uuid.Parse(task.Owner); err == nil {
			ownerUUID = &parsed
		}
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE dispatch_tasks SET
			requester = $2, owner = $3, submitter = $4, title = $5, description = $6, scope = $7, priority = $8,
			status = $9, assignee = $10, result = $11, error = $12, context = $13,
			timeout_ms = $14, max_retries = $15, retry_count = $16, parent_id = $17,
			assigned_at = $18, started_at = $19, completed_at = $20
		WHERE id = $1`,
		task.ID, task.Requester, ownerUUID, task.Submitter, task.Title, task.Description, task.Scope, task.Priority,
		task.Status, task.Assignee, resultJSON, task.Error, contextJSON,
		task.TimeoutMs, task.MaxRetries, task.RetryCount, task.ParentID,
		task.AssignedAt, task.StartedAt, task.CompletedAt,
	)
	return err
}

func (s *PostgresStore) CreateTaskEvent(ctx context.Context, event *TaskEvent) error {
	payloadJSON, _ := json.Marshal(event.Payload)
	return s.pool.QueryRow(ctx, `
		INSERT INTO dispatch_task_events (task_id, event, agent_id, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		event.TaskID, event.Event, event.AgentID, payloadJSON,
	).Scan(&event.ID, &event.CreatedAt)
}

func (s *PostgresStore) GetTaskEvents(ctx context.Context, taskID uuid.UUID) ([]*TaskEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, task_id, event, agent_id, payload, created_at
		FROM dispatch_task_events WHERE task_id = $1
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
			COALESCE(SUM(CASE WHEN status IN ('assigned','running') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(EXTRACT(EPOCH FROM (completed_at - assigned_at)) * 1000) FILTER (WHERE status = 'completed' AND completed_at IS NOT NULL AND assigned_at IS NOT NULL), 0)
		FROM dispatch_tasks`,
	).Scan(&stats.TotalPending, &stats.TotalRunning, &stats.TotalCompleted, &stats.TotalFailed, &stats.AvgCompletionMs)
	return stats, err
}

func scanTasks(rows pgx.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		var resultJSON, contextJSON []byte
		var ownerUUID *uuid.UUID
		if err := rows.Scan(
			&t.ID, &t.Requester, &ownerUUID, &t.Submitter, &t.Title, &t.Description, &t.Scope, &t.Priority,
			&t.Status, &t.Assignee, &resultJSON, &t.Error, &contextJSON,
			&t.TimeoutMs, &t.MaxRetries, &t.RetryCount, &t.ParentID,
			&t.CreatedAt, &t.AssignedAt, &t.StartedAt, &t.CompletedAt,
		); err != nil {
			return nil, err
		}
		if ownerUUID != nil {
			t.Owner = ownerUUID.String()
		}
		if resultJSON != nil {
			_ = json.Unmarshal(resultJSON, &t.Result)
		}
		if contextJSON != nil {
			_ = json.Unmarshal(contextJSON, &t.Context)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
