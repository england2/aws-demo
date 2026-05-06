package main

import "time"

// AgentJobStatus is the durable state enum shared by SQS messages and agent jobs.
// These values are stored as text in SQLite and drive conductor decisions.
type AgentJobStatus string

const (
	AgentJobStatusCreated   AgentJobStatus = "created"
	AgentJobStatusRunning   AgentJobStatus = "running"
	AgentJobStatusSucceeded AgentJobStatus = "succeeded"
	AgentJobStatusFailed    AgentJobStatus = "failed"
	AgentJobStatusIgnored   AgentJobStatus = "ignored"
	AgentJobStatusDuplicate AgentJobStatus = "duplicate"
)

const databaseDir = "go-agent-conductor-runtime-database"

// DatabaseSQSMessageInfo is the conductor's domain view of inbound SQS messages.
// It combines transport fields, parsed CloudWatch fields, and job assignment state.
// The struct bridges poll.go, sqlc row conversion, and database decision logic.
type DatabaseSQSMessageInfo struct {
	ID                  int64
	ExternalMessageID   string
	ReceiptHandle       string
	ExternalEventID     *string
	Body                []byte
	RawBody             string
	MessageType         string
	CloudWatchAlarmName *string
	CloudWatchState     *string
	EventTime           *time.Time
	AlarmPeriodSeconds  *int64
	AssignedAgentJobID  *int64
	JobStatus           *AgentJobStatus
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// DatabaseAgentJobInfo is the conductor's durable view of one Fargate/Codex job.
// It is created from an actionable inbound message and updated as ECS/runtime state changes.
// It intentionally carries debug/report fields for post-run inspection.
type DatabaseAgentJobInfo struct {
	ID                int64
	AgentName         string
	Status            AgentJobStatus
	SpawnSQSMessageID int64
	AgentReport       *string
	AffectedRepos     *string
	PullRequestURL    *string
	FailureReason     *string
	ECSTaskARN        *string
	ECSLastStatus     *string
	ECSStoppedReason  *string
	CreatedAt         time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	UpdatedAt         time.Time
}

// DatabaseQuarantinedSQSMessageInfo describes malformed or unusable SQS input.
// The conductor writes these rows before deleting poison messages from their queues.
type DatabaseQuarantinedSQSMessageInfo struct {
	ID                int64
	QueueSource       string
	ExternalMessageID *string
	ReceiptHandle     *string
	RawBody           string
	QuarantineReason  string
	CreatedAt         time.Time
}
