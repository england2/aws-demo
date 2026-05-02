package lib

import "time"

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

type AgentEvent struct {
	JobID       string         `json:"job_id"`
	AgentName   string         `json:"agent_name"`
	Type        AgentEventType `json:"type"`
	Message     string         `json:"message,omitempty"`
	ReportPath  string         `json:"report_path,omitempty"`
	ArtifactURL string         `json:"artifact_url,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}
