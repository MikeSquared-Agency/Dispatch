-- 002_swarm_tasks_state_machine.sql
-- Migrate dispatch_tasks → swarm_tasks with full state machine schema.
--
-- States: pending, assigned, in_progress, completed, failed, timed_out
-- See docs/task-state-machine.md for the full contract.

BEGIN;

-- 1. Drop existing indexes
DROP INDEX IF EXISTS idx_dispatch_tasks_status;
DROP INDEX IF EXISTS idx_dispatch_tasks_requester;
DROP INDEX IF EXISTS idx_dispatch_tasks_assignee;
DROP INDEX IF EXISTS idx_dispatch_tasks_scope;
DROP INDEX IF EXISTS idx_dispatch_tasks_priority;
DROP INDEX IF EXISTS idx_dispatch_tasks_parent;
DROP INDEX IF EXISTS idx_dispatch_tasks_owner;
DROP INDEX IF EXISTS idx_dispatch_events_task;
DROP INDEX IF EXISTS idx_dispatch_events_created;

-- 2. Drop foreign keys before structural changes
ALTER TABLE dispatch_task_events
  DROP CONSTRAINT IF EXISTS dispatch_task_events_task_id_fkey;
ALTER TABLE dispatch_tasks
  DROP CONSTRAINT IF EXISTS dispatch_tasks_parent_id_fkey;

-- 3. Drop old check constraints
ALTER TABLE dispatch_tasks
  DROP CONSTRAINT IF EXISTS dispatch_tasks_priority_check;
ALTER TABLE dispatch_tasks
  DROP CONSTRAINT IF EXISTS dispatch_tasks_status_check;

-- 4. Rename tables
ALTER TABLE dispatch_tasks RENAME TO swarm_tasks;
ALTER TABLE dispatch_task_events RENAME TO swarm_task_events;

-- 5. Rename columns
ALTER TABLE swarm_tasks RENAME COLUMN id TO task_id;
ALTER TABLE swarm_tasks RENAME COLUMN assignee TO assigned_agent;
ALTER TABLE swarm_tasks RENAME COLUMN parent_id TO parent_task_id;

-- 6. Convert timeout_ms → timeout_seconds
ALTER TABLE swarm_tasks RENAME COLUMN timeout_ms TO timeout_seconds;
UPDATE swarm_tasks SET timeout_seconds = COALESCE(timeout_seconds / 1000, 300);
ALTER TABLE swarm_tasks ALTER COLUMN timeout_seconds SET DEFAULT 300;

-- 7. Add source column and migrate from submitter
ALTER TABLE swarm_tasks ADD COLUMN IF NOT EXISTS source text DEFAULT 'manual';
UPDATE swarm_tasks
  SET source = submitter
  WHERE submitter IS NOT NULL AND submitter != '';

-- 8. Add metadata column and migrate from context
ALTER TABLE swarm_tasks ADD COLUMN IF NOT EXISTS metadata jsonb DEFAULT '{}';
UPDATE swarm_tasks
  SET metadata = context
  WHERE context IS NOT NULL;

-- 9. Add remaining new columns
ALTER TABLE swarm_tasks ADD COLUMN IF NOT EXISTS required_capabilities text[] DEFAULT '{}';
ALTER TABLE swarm_tasks ADD COLUMN IF NOT EXISTS updated_at timestamptz DEFAULT now();
ALTER TABLE swarm_tasks ADD COLUMN IF NOT EXISTS retry_eligible boolean DEFAULT true;

-- 10. Migrate scope → required_capabilities
UPDATE swarm_tasks
  SET required_capabilities = ARRAY[scope]
  WHERE scope IS NOT NULL AND scope != '';

-- 11. Migrate status values
UPDATE swarm_tasks SET status = 'in_progress' WHERE status = 'running';
UPDATE swarm_tasks SET status = 'timed_out'   WHERE status = 'timeout';
UPDATE swarm_tasks SET status = 'failed'      WHERE status = 'cancelled';

-- 12. Change owner from UUID to TEXT
ALTER TABLE swarm_tasks ALTER COLUMN owner TYPE text USING COALESCE(owner::text, 'system');
UPDATE swarm_tasks SET owner = 'system' WHERE owner IS NULL OR owner = '';
ALTER TABLE swarm_tasks ALTER COLUMN owner SET NOT NULL;

-- 13. Drop old columns
ALTER TABLE swarm_tasks DROP COLUMN IF EXISTS requester;
ALTER TABLE swarm_tasks DROP COLUMN IF EXISTS scope;
ALTER TABLE swarm_tasks DROP COLUMN IF EXISTS context;
ALTER TABLE swarm_tasks DROP COLUMN IF EXISTS submitter;

-- 14. Update defaults
ALTER TABLE swarm_tasks ALTER COLUMN max_retries SET DEFAULT 3;
ALTER TABLE swarm_tasks ALTER COLUMN priority SET DEFAULT 0;

-- 15. Add new constraints
ALTER TABLE swarm_tasks ADD CONSTRAINT valid_status CHECK (
  status IN ('pending', 'assigned', 'in_progress', 'completed', 'failed', 'timed_out')
);
ALTER TABLE swarm_tasks ADD CONSTRAINT valid_priority CHECK (
  priority >= 0 AND priority <= 10
);

-- 16. Recreate foreign keys
ALTER TABLE swarm_tasks ADD CONSTRAINT swarm_tasks_parent_fk
  FOREIGN KEY (parent_task_id) REFERENCES swarm_tasks(task_id);
ALTER TABLE swarm_task_events ADD CONSTRAINT swarm_task_events_task_fk
  FOREIGN KEY (task_id) REFERENCES swarm_tasks(task_id);

-- 17. Create indexes (matching spec)
CREATE INDEX idx_tasks_status   ON swarm_tasks (status);
CREATE INDEX idx_tasks_agent    ON swarm_tasks (assigned_agent);
CREATE INDEX idx_tasks_owner    ON swarm_tasks (owner);
CREATE INDEX idx_tasks_created  ON swarm_tasks (created_at DESC);
CREATE INDEX idx_tasks_parent   ON swarm_tasks (parent_task_id);
CREATE INDEX idx_tasks_priority ON swarm_tasks (priority DESC, created_at ASC);

-- Timeout watcher indexes (partial)
CREATE INDEX idx_tasks_assigned_timeout
  ON swarm_tasks (assigned_at)
  WHERE status = 'assigned';

CREATE INDEX idx_tasks_started_timeout
  ON swarm_tasks (started_at)
  WHERE status = 'in_progress';

-- Event indexes
CREATE INDEX idx_task_events_task    ON swarm_task_events (task_id);
CREATE INDEX idx_task_events_created ON swarm_task_events (created_at);

-- 18. Auto-update updated_at trigger
CREATE OR REPLACE FUNCTION update_swarm_tasks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER swarm_tasks_updated_at
  BEFORE UPDATE ON swarm_tasks
  FOR EACH ROW
  EXECUTE FUNCTION update_swarm_tasks_updated_at();

COMMIT;
