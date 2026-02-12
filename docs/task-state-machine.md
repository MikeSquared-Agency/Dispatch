# Task State Machine

Dispatch manages tasks through a formal state machine backed by the `swarm_tasks` table in Supabase and NATS JetStream for event distribution.

## States

| Status | Description |
|--------|-------------|
| `pending` | Task created, awaiting assignment |
| `assigned` | Broker matched task to an agent |
| `in_progress` | Agent has started execution |
| `completed` | Agent reported successful completion |
| `failed` | Agent reported failure (may retry) |
| `timed_out` | Timeout watcher detected deadline exceeded (may retry) |

## Transitions

```
pending ──────────► assigned ──────────► in_progress
  ▲                                        │
  │              (retry)                   │
  ├────────────────────────────────────────┤
  │                                        │
  │                                        ├──► completed
  │                                        │
  │                                        ├──► failed ──► DLQ (if exhausted)
  │                                        │
  │                                        └──► timed_out ──► DLQ (if exhausted)
  │                                                │
  └────────────────────────────────────────────────┘
                   (retry)
```

### Valid Transitions

| From | To | Triggered By |
|------|----|-------------|
| `pending` | `assigned` | Broker assignment loop |
| `assigned` | `in_progress` | Agent progress report or started event |
| `assigned` | `timed_out` | Timeout watcher (no agent acknowledgement) |
| `in_progress` | `completed` | Agent completion report |
| `in_progress` | `failed` | Agent failure report |
| `in_progress` | `timed_out` | Timeout watcher (deadline exceeded) |
| `failed` | `pending` | Retry (if `retry_eligible` and `retry_count < max_retries`) |
| `timed_out` | `pending` | Retry (if retries remain) |

### Retry Logic

When a task fails or times out:

1. Check `retry_eligible` flag (agents set this on failure reports)
2. Check `retry_count < max_retries`
3. If both true: reset to `pending`, increment `retry_count`, clear assignment fields
4. If either false: mark as terminal failure, publish to DLQ

Default `max_retries` is 3. Default `retry_eligible` is `true`.

### Dead Letter Queue (DLQ)

Tasks that exhaust retries or are marked non-retryable are published to `swarm.task.<id>.dlq`. These require manual intervention or automated escalation.

## Schema

### `swarm_tasks` Table

| Column | Type | Description |
|--------|------|-------------|
| `task_id` | `uuid` | Primary key (auto-generated) |
| `title` | `text` | Human-readable task title (required) |
| `description` | `text` | Detailed task description |
| `owner` | `text` | Task owner (user or agent ID) |
| `required_capabilities` | `text[]` | Capabilities the assigned agent must have |
| `status` | `text` | Current state (see States above) |
| `assigned_agent` | `text` | ID of the assigned agent |
| `created_at` | `timestamptz` | When the task was created |
| `assigned_at` | `timestamptz` | When the task was assigned |
| `started_at` | `timestamptz` | When execution began |
| `completed_at` | `timestamptz` | When the task reached a terminal state |
| `updated_at` | `timestamptz` | Last modification timestamp (auto-updated) |
| `result` | `jsonb` | Completion result payload |
| `error` | `text` | Error message on failure |
| `retry_count` | `integer` | Number of retries attempted |
| `max_retries` | `integer` | Maximum retry attempts allowed (default: 3) |
| `retry_eligible` | `boolean` | Whether the task can be retried (default: true) |
| `timeout_seconds` | `integer` | Execution deadline in seconds (default: 300) |
| `priority` | `integer` | Priority 0-10, higher = more urgent (default: 0) |
| `source` | `text` | Origin of the task (e.g. `agent`, `manual`, `nats`) |
| `parent_task_id` | `uuid` | Parent task for sub-task hierarchies |
| `metadata` | `jsonb` | Arbitrary key-value metadata |

### `swarm_task_events` Table

| Column | Type | Description |
|--------|------|-------------|
| `id` | `uuid` | Primary key |
| `task_id` | `uuid` | Foreign key to `swarm_tasks` |
| `event` | `text` | Event type (created, assigned, started, completed, failed, timeout, retry, dlq) |
| `agent_id` | `text` | Agent that triggered the event |
| `payload` | `jsonb` | Event-specific data |
| `created_at` | `timestamptz` | Event timestamp |

## NATS Subjects

All task events are published to NATS JetStream subjects following the pattern:

```
swarm.task.<task_id>.<event>
```

| Subject | Published When |
|---------|---------------|
| `swarm.task.<id>.created` | Task is created |
| `swarm.task.<id>.assigned` | Broker assigns task to agent |
| `swarm.task.<id>.started` | Agent begins execution |
| `swarm.task.<id>.progress` | Agent reports progress |
| `swarm.task.<id>.completed` | Agent completes task |
| `swarm.task.<id>.failed` | Agent reports failure |
| `swarm.task.<id>.timeout` | Timeout watcher fires |
| `swarm.task.<id>.retry` | Task is retried (reset to pending) |
| `swarm.task.<id>.dlq` | Task sent to dead letter queue |

## API Endpoints

All endpoints require `X-Agent-ID` header. Admin endpoints also require `Authorization: Bearer <token>`.

### Task Operations

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/tasks` | Create a new task |
| `GET` | `/api/v1/tasks` | List tasks (supports `?status=`, `?owner=`, `?source=`, `?agent=` filters) |
| `GET` | `/api/v1/tasks/:id` | Get a specific task |
| `PATCH` | `/api/v1/tasks/:id` | Update task metadata |
| `POST` | `/api/v1/tasks/:id/complete` | Mark task completed with result |
| `POST` | `/api/v1/tasks/:id/fail` | Mark task failed with error |
| `POST` | `/api/v1/tasks/:id/progress` | Report progress (transitions assigned -> in_progress) |

### Admin Operations

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/stats` | Get task statistics |
| `GET` | `/api/v1/agents` | List agents with capabilities and active tasks |
| `POST` | `/api/v1/agents/:id/drain` | Drain an agent (stop new assignments) |

### Create Task Request

```json
{
  "title": "Research topic X",
  "description": "Detailed description",
  "owner": "mike-d",
  "required_capabilities": ["research", "analysis"],
  "priority": 7,
  "timeout_seconds": 600,
  "max_retries": 5,
  "source": "manual",
  "parent_task_id": "uuid-of-parent",
  "metadata": {"key": "value"}
}
```

Only `title` is required. Defaults: `owner` = agent ID from header, `source` = "agent", `timeout_seconds` = 300, `max_retries` = 3, `retry_eligible` = true.

### Complete Task Request

```json
{
  "result": {"output": "findings", "pages": 42}
}
```

### Fail Task Request

```json
{
  "error": "connection refused",
  "retry_eligible": false
}
```

## Priority System

Priority ranges from 0 (lowest) to 10 (highest). The broker uses priority in two ways:

1. **Queue ordering**: `ORDER BY priority DESC, created_at ASC` (highest priority, oldest first)
2. **Candidate scoring**: Priority weight = `1.0 + priority * 0.05` (range 1.0 to 1.5)

## Capability Matching

Tasks specify `required_capabilities` (e.g. `["research", "analysis"]`). The broker matches these against agent personas from Forge. All required capabilities must be present (case-insensitive) for a match score > 0.

## Timeout Watcher

The broker runs a timeout check every 30 seconds:

1. Queries all `assigned` and `in_progress` tasks
2. For `assigned` tasks: checks `assigned_at + timeout_seconds`
3. For `in_progress` tasks: checks `started_at + timeout_seconds` (falls back to `assigned_at`)
4. Timed-out tasks follow the retry/DLQ logic described above

## Ownership Model

- **Dispatch** (broker) owns: `pending -> assigned`, timeout detection, retry/DLQ decisions
- **Agents** own: `assigned -> in_progress`, `in_progress -> completed/failed`, setting `retry_eligible`
