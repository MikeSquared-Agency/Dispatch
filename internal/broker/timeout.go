package broker

import (
	"context"
	"time"

	"github.com/DarlingtonDeveloper/Dispatch/internal/hermes"
	"github.com/DarlingtonDeveloper/Dispatch/internal/store"
)

func (b *Broker) timeoutLoop(ctx context.Context) {
	defer b.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.checkTimeouts(ctx)
		}
	}
}

func (b *Broker) checkTimeouts(ctx context.Context) {
	tasks, err := b.store.GetActiveTasks(ctx)
	if err != nil {
		b.logger.Error("failed to get active tasks for timeout check", "error", err)
		return
	}

	now := time.Now()
	for _, task := range tasks {
		var start time.Time
		if task.StartedAt != nil {
			start = *task.StartedAt
		} else if task.AssignedAt != nil {
			start = *task.AssignedAt
		} else {
			continue
		}

		timeout := time.Duration(task.TimeoutSeconds) * time.Second
		if now.Sub(start) <= timeout {
			continue
		}

		timedOutIn := string(task.Status)
		b.logger.Warn("task timed out", "task_id", task.ID, "assigned_agent", task.AssignedAgent, "timed_out_in", timedOutIn)

		if task.RetryCount < task.MaxRetries {
			// Retry — reset to pending for re-assignment
			task.RetryCount++
			task.Status = store.StatusPending
			task.AssignedAgent = ""
			task.AssignedAt = nil
			task.StartedAt = nil
			if err := b.store.UpdateTask(ctx, task); err != nil {
				b.logger.Error("failed to reset timed out task", "task_id", task.ID, "error", err)
				continue
			}
			_ = b.store.CreateTaskEvent(ctx, &store.TaskEvent{
				TaskID: task.ID,
				Event:  "timeout_retry",
			})
			if b.hermes != nil {
				_ = b.hermes.Publish(hermes.SubjectTaskTimeout(task.ID.String()), hermes.TaskTimeoutEvent{
					TaskID:     task.ID.String(),
					RetryCount: task.RetryCount,
					MaxRetries: task.MaxRetries,
					TimedOutIn: timedOutIn,
				})
				_ = b.hermes.Publish(hermes.SubjectTaskRetry(task.ID.String()), map[string]interface{}{
					"task_id":        task.ID.String(),
					"retry_count":    task.RetryCount,
					"max_retries":    task.MaxRetries,
					"previous_state": timedOutIn,
					"previous_agent": task.AssignedAgent,
				})
			}
		} else {
			// Exhausted — mark timed_out and DLQ
			completedAt := now
			task.Status = store.StatusTimedOut
			task.CompletedAt = &completedAt
			task.Error = "task timed out after all retries"
			if err := b.store.UpdateTask(ctx, task); err != nil {
				b.logger.Error("failed to mark task as timed out", "task_id", task.ID, "error", err)
				continue
			}
			_ = b.store.CreateTaskEvent(ctx, &store.TaskEvent{
				TaskID: task.ID,
				Event:  "timeout_exhausted",
			})
			if b.hermes != nil {
				_ = b.hermes.Publish(hermes.SubjectTaskTimeout(task.ID.String()), hermes.TaskTimeoutEvent{
					TaskID:     task.ID.String(),
					RetryCount: task.RetryCount,
					MaxRetries: task.MaxRetries,
					TimedOutIn: timedOutIn,
				})
				_ = b.hermes.Publish(hermes.SubjectTaskDLQ(task.ID.String()), map[string]interface{}{
					"task_id":     task.ID.String(),
					"reason":      "timeout_exhausted",
					"retry_count": task.RetryCount,
					"max_retries": task.MaxRetries,
				})
			}
		}
	}
}
