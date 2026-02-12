package hermes

const (
	SubjectTaskRequest     = "swarm.task.request"
	SubjectAgentStarted    = "swarm.agent.*.started"
	SubjectAgentStopped    = "swarm.agent.*.stopped"
	SubjectDispatchStats   = "swarm.dispatch.stats"

	StreamName    = "DISPATCH_EVENTS"
	StreamMaxAge  = "720h" // 30 days
)

func SubjectTaskAssigned(taskID string) string   { return "swarm.task." + taskID + ".assigned" }
func SubjectTaskCompleted(taskID string) string   { return "swarm.task." + taskID + ".completed" }
func SubjectTaskFailed(taskID string) string      { return "swarm.task." + taskID + ".failed" }
func SubjectTaskReassigned(taskID string) string  { return "swarm.task." + taskID + ".reassigned" }
func SubjectTaskTimeout(taskID string) string     { return "swarm.task." + taskID + ".timeout" }
func SubjectTaskProgress(taskID string) string    { return "swarm.task." + taskID + ".progress" }
func SubjectTaskUnmatched(taskID string) string   { return "swarm.task." + taskID + ".unmatched" }
