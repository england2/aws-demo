package main

import (
	"database/sql"
	"time"

	dbgen "agent-orchestrator/internal/db/generated"
)

// Adapters from sqlc row models to conductor domain models while the database layer is being migrated.

// databaseSQSMessageFromGenerated converts a sqlc SQS row into the conductor domain type.
// It centralizes nullable/time parsing so intake code can operate on plain pointers.
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

// databaseAgentJobFromGenerated converts a sqlc agent_job_info row into domain state.
// Agent lifecycle code uses this after generated INSERT/UPDATE ... RETURNING queries.
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

// databaseQuarantinedSQSMessageFromGenerated converts a quarantine sqlc row to domain state.
// The router returns this after storing malformed SQS messages for later debugging.
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

// nullableString adapts optional strings into database/sql compatible values.
// It is used by manual SQL paths that have not yet moved fully into sqlc params.
func nullableString(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}

// nullableTime adapts optional time values to the SQLite text timestamp format.
// Nil inputs become SQL NULL; non-nil values are normalized to UTC RFC3339Nano.
func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC().Format(time.RFC3339Nano)
}

// nullableInt64 adapts optional integer fields into SQL-compatible values.
// It is primarily used for parsed CloudWatch metric periods.
func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}

	return *value
}

// nullablePlainString treats an empty string as SQL NULL.
// This is useful for optional transport metadata such as receipt handles or event IDs.
func nullablePlainString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

// nullableNonZeroTime stores zero time as SQL NULL and real times as UTC text.
// Agent event ingestion uses this for optional event-created timestamps.
func nullableNonZeroTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}

	return value.UTC().Format(time.RFC3339Nano)
}

// sqlNullString builds a sql.NullString with Valid=false for empty strings.
// Generated sqlc parameter structs use this for nullable text columns.
func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// stringFromNull converts sql.NullString to the pointer style used by domain structs.
// Keeping this helper small avoids repeated Valid checks across conversion code.
func stringFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	return &value.String
}

// int64FromNull converts nullable SQL integers to optional Go integers.
// It is used for IDs, metric periods, and other nullable numeric columns.
func int64FromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}

	return &value.Int64
}

// agentJobStatusFromNull converts nullable text into the shared status enum pointer.
// This keeps message-level status reads consistent with agent_job_info statuses.
func agentJobStatusFromNull(value sql.NullString) *AgentJobStatus {
	if !value.Valid {
		return nil
	}

	status := AgentJobStatus(value.String)
	return &status
}

// timeFromNull converts nullable SQLite timestamp text into optional time values.
// Invalid non-null timestamps panic through mustParseDatabaseTime to catch schema bugs.
func timeFromNull(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}

	parsed := mustParseDatabaseTime(value.String)
	return &parsed
}

// mustParseDatabaseTime parses a DB timestamp and panics on invalid stored values.
// The database layer treats invalid persisted timestamps as programmer/schema bugs.
func mustParseDatabaseTime(value string) time.Time {
	parsed, err := parseDatabaseTime(value)
	if err != nil {
		return time.Time{}
	}

	return parsed
}

// parseDatabaseTime accepts both RFC3339Nano and SQLite CURRENT_TIMESTAMP formats.
// This bridges explicit app-written timestamps and SQLite default timestamp values.
func parseDatabaseTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}

	return time.Parse("2006-01-02 15:04:05", value)
}
