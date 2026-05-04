package main

import "time"

type AgentJobStatus string

const (
	AgentJobStatusCreated   AgentJobStatus = "created"
	AgentJobStatusRunning   AgentJobStatus = "running"
	AgentJobStatusSucceeded AgentJobStatus = "succeeded"
	AgentJobStatusFailed    AgentJobStatus = "failed"
	AgentJobStatusIgnored   AgentJobStatus = "ignored"
	AgentJobStatusDuplicate AgentJobStatus = "duplicate"
)

const databaseDir = "database"

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

type DatabaseAgentEventInfo struct {
	ID          int64
	EventID     string
	AgentJobID  int64
	AgentName   string
	EventType   string
	Message     *string
	ReportPath  *string
	ArtifactURL *string
	RawBody     string
	CreatedAt   *time.Time
	ReceivedAt  time.Time
}
