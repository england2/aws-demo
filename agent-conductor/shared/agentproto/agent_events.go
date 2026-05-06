package agentproto

type AgentEventType string

const (
	AgentWrapperStarted  AgentEventType = "AgentWrapperStarted"
	AgentSetupStarted    AgentEventType = "AgentSetupStarted"
	AgentSetupFailed     AgentEventType = "AgentSetupFailed"
	CodexStarted         AgentEventType = "CodexStarted"
	CodexExited          AgentEventType = "CodexExited"
	AgentReportedSuccess AgentEventType = "AgentReportedSuccess"
	AgentReportedFailure AgentEventType = "AgentReportedFailure"
	PullRequestCreated   AgentEventType = "PullRequestCreated"
	JobCompleted         AgentEventType = "JobCompleted"
	JobFailed            AgentEventType = "JobFailed"
)

// ai--done
// we're restructing agentevents to where they are control objects only!
// we will not record them and will not wrap them in a struct.
// please adjust the codebase accordingly
type AgentEvent struct {
	JobID     string         `json:"job_id"`
	AgentName string         `json:"agent_name"`
	Type      AgentEventType `json:"type"`
}
