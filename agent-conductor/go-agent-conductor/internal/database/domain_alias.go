package database

import (
	"agent-orchestrator/internal/domain"
	"agent-orchestrator/internal/messages"
)

// These aliases keep the database transaction code readable while domain/message
// ownership moves out of package main. They are intentionally narrow transition glue.
type DatabaseSQSMessageInfo = domain.DatabaseSQSMessageInfo
type DatabaseAgentJobInfo = domain.DatabaseAgentJobInfo
type DatabaseAgentEventInfo = domain.DatabaseAgentEventInfo
type DatabaseQuarantinedSQSMessageInfo = domain.DatabaseQuarantinedSQSMessageInfo
type AgentJobStatus = domain.AgentJobStatus
type ParsedSQSMessage = messages.ParsedSQSMessage

const (
	AgentJobStatusCreated      = domain.AgentJobStatusCreated
	AgentJobStatusRunning      = domain.AgentJobStatusRunning
	AgentJobStatusSucceeded    = domain.AgentJobStatusSucceeded
	AgentJobStatusFailed       = domain.AgentJobStatusFailed
	AgentJobStatusIgnored      = domain.AgentJobStatusIgnored
	AgentJobStatusDuplicate    = domain.AgentJobStatusDuplicate
	MessageTypeUnknown         = messages.MessageTypeUnknown
	MessageTypeCloudWatchAlarm = messages.MessageTypeCloudWatchAlarm
)

// ParseSQSMessageBody delegates classification to the messages package.
func ParseSQSMessageBody(body []byte) messages.ParsedSQSMessage {
	return messages.ParseSQSMessageBody(body)
}
