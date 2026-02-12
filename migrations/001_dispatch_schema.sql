CREATE TABLE IF NOT EXISTS dispatch_tasks (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  requester     TEXT NOT NULL,
  title         TEXT NOT NULL,
  description   TEXT,
  scope         TEXT NOT NULL,
  priority      INTEGER DEFAULT 3 CHECK (priority BETWEEN 1 AND 5),
  status        TEXT DEFAULT 'pending' CHECK (status IN ('pending','assigned','running','completed','failed','cancelled','timeout')),
  assignee      TEXT,
  result        JSONB,
  error         TEXT,
  context       JSONB,
  timeout_ms    INTEGER DEFAULT 300000,
  max_retries   INTEGER DEFAULT 1,
  retry_count   INTEGER DEFAULT 0,
  parent_id     UUID REFERENCES dispatch_tasks(id),
  created_at    TIMESTAMPTZ DEFAULT now(),
  assigned_at   TIMESTAMPTZ,
  started_at    TIMESTAMPTZ,
  completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_status ON dispatch_tasks(status);
CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_requester ON dispatch_tasks(requester);
CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_assignee ON dispatch_tasks(assignee);
CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_scope ON dispatch_tasks(scope);
CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_priority ON dispatch_tasks(priority);
CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_parent ON dispatch_tasks(parent_id);

CREATE TABLE IF NOT EXISTS dispatch_task_events (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id    UUID NOT NULL REFERENCES dispatch_tasks(id),
  event      TEXT NOT NULL,
  agent_id   TEXT,
  payload    JSONB,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dispatch_events_task ON dispatch_task_events(task_id);
CREATE INDEX IF NOT EXISTS idx_dispatch_events_created ON dispatch_task_events(created_at);
