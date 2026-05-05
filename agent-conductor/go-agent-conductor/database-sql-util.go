package main

import (
	"database/sql"
	"time"

	dbgen "agent-orchestrator/internal/db/generated"
)

// Adapters from sqlc row models to conductor domain models while the database layer is being migrated.

func databaseSQSMessageFromGenerated(message dbgen.SqsMessagesTicketsCloudwatch) DatabaseSQSMessageInfo {
	return DatabaseSQSMessageInfo{
		ID:                  message.ID,
		ExternalMessageID:   message.ExternalMessageID,
		ReceiptHandle:       message.ReceiptHandle,
		ExternalEventID:     stringFromNull(message.ExternalEventID),
		Body:                []byte(message.RawBody),
		RawBody:             message.RawBody,
		MessageType:         message.MessageType,
		CloudWatchAlarmName: stringFromNull(message.CloudwatchAlarmName),
		CloudWatchState:     stringFromNull(message.CloudwatchState),
		EventTime:           timeFromNull(message.EventTime),
		AlarmPeriodSeconds:  int64FromNull(message.AlarmPeriodSeconds),
		AssignedAgentJobID:  int64FromNull(message.AssignedAgentJobID),
		JobStatus:           agentJobStatusFromNull(message.JobStatus),
		CreatedAt:           mustParseDatabaseTime(message.CreatedAt),
		UpdatedAt:           mustParseDatabaseTime(message.UpdatedAt),
	}
}

func databaseAgentJobFromGenerated(agentJob dbgen.AgentJobInfo) DatabaseAgentJobInfo {
	return DatabaseAgentJobInfo{
		ID:                agentJob.ID,
		AgentName:         agentJob.AgentName,
		Status:            AgentJobStatus(agentJob.Status),
		SpawnSQSMessageID: agentJob.SpawnSqsMessageID,
		AgentReport:       stringFromNull(agentJob.AgentReport),
		AffectedRepos:     stringFromNull(agentJob.AffectedRepositories),
		PullRequestURL:    stringFromNull(agentJob.PullRequestUrl),
		FailureReason:     stringFromNull(agentJob.FailureReason),
		ECSTaskARN:        stringFromNull(agentJob.EcsTaskArn),
		ECSLastStatus:     stringFromNull(agentJob.EcsLastStatus),
		ECSStoppedReason:  stringFromNull(agentJob.EcsStoppedReason),
		CreatedAt:         mustParseDatabaseTime(agentJob.CreatedAt),
		StartedAt:         timeFromNull(agentJob.StartedAt),
		CompletedAt:       timeFromNull(agentJob.CompletedAt),
		UpdatedAt:         mustParseDatabaseTime(agentJob.UpdatedAt),
	}
}

func databaseAgentEventFromGenerated(event dbgen.AgentEvent) DatabaseAgentEventInfo {
	return DatabaseAgentEventInfo{
		ID:          event.ID,
		EventID:     event.EventID,
		AgentJobID:  event.AgentJobID,
		AgentName:   event.AgentName,
		EventType:   event.EventType,
		Message:     stringFromNull(event.Message),
		ReportPath:  stringFromNull(event.ReportPath),
		ArtifactURL: stringFromNull(event.ArtifactUrl),
		RawBody:     event.RawBody,
		CreatedAt:   timeFromNull(event.CreatedAt),
		ReceivedAt:  mustParseDatabaseTime(event.ReceivedAt),
	}
}

func databaseQuarantinedSQSMessageFromGenerated(message dbgen.QuarantinedSqsMessage) DatabaseQuarantinedSQSMessageInfo {
	return DatabaseQuarantinedSQSMessageInfo{
		ID:                message.ID,
		QueueSource:       message.QueueSource,
		ExternalMessageID: stringFromNull(message.ExternalMessageID),
		ReceiptHandle:     stringFromNull(message.ReceiptHandle),
		RawBody:           message.RawBody,
		QuarantineReason:  message.QuarantineReason,
		CreatedAt:         mustParseDatabaseTime(message.CreatedAt),
	}
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC().Format(time.RFC3339Nano)
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}

	return *value
}

func nullablePlainString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func nullableNonZeroTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}

	return value.UTC().Format(time.RFC3339Nano)
}

func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func stringFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	return &value.String
}

func int64FromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}

	return &value.Int64
}

func agentJobStatusFromNull(value sql.NullString) *AgentJobStatus {
	if !value.Valid {
		return nil
	}

	status := AgentJobStatus(value.String)
	return &status
}

func timeFromNull(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}

	parsed := mustParseDatabaseTime(value.String)
	return &parsed
}

func mustParseDatabaseTime(value string) time.Time {
	parsed, err := parseDatabaseTime(value)
	if err != nil {
		return time.Time{}
	}

	return parsed
}

func parseDatabaseTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}

	return time.Parse("2006-01-02 15:04:05", value)
}
