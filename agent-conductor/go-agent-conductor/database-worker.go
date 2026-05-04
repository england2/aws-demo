package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const defaultAgentJobName = "agent-fargate-codex"

type DatabaseCommandKind string

const (
	DatabaseCommandRecordSQSMessage DatabaseCommandKind = "record_sqs_message"
)

type DatabaseCommand struct {
	Kind       DatabaseCommandKind
	SQSMessage DatabaseSQSMessageInfo
	Reply      chan DatabaseCommandResult
}

type DatabaseCommandResult struct {
	Message             DatabaseSQSMessageInfo
	AgentJob            *DatabaseAgentJobInfo
	ShouldSpawnAgentJob bool
	DeleteSQSMessage    bool
	Reason              string
	Err                 error
}

func StartDatabaseWorker(ctx context.Context) (chan<- DatabaseCommand, error) {
	db, err := openConductorDB()
	if err != nil {
		return nil, err
	}

	commands := make(chan DatabaseCommand)
	go func() {
		defer db.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case command, ok := <-commands:
				if !ok {
					return
				}
				handleDatabaseCommand(ctx, db, command)
			}
		}
	}()

	return commands, nil
}

func RecordSQSMessageWithDatabase(ctx context.Context, commands chan<- DatabaseCommand, message DatabaseSQSMessageInfo) DatabaseCommandResult {
	reply := make(chan DatabaseCommandResult, 1)
	command := DatabaseCommand{
		Kind:       DatabaseCommandRecordSQSMessage,
		SQSMessage: message,
		Reply:      reply,
	}

	select {
	case commands <- command:
	case <-ctx.Done():
		return DatabaseCommandResult{Err: ctx.Err()}
	}

	select {
	case result := <-reply:
		return result
	case <-ctx.Done():
		return DatabaseCommandResult{Err: ctx.Err()}
	}
}

func handleDatabaseCommand(ctx context.Context, db *sql.DB, command DatabaseCommand) {
	var result DatabaseCommandResult

	switch command.Kind {
	case DatabaseCommandRecordSQSMessage:
		result = recordSQSMessageAndDecide(ctx, db, command.SQSMessage)
	default:
		result = DatabaseCommandResult{Err: fmt.Errorf("unknown database command kind %q", command.Kind)}
	}

	select {
	case command.Reply <- result:
	case <-ctx.Done():
	}
}

func openConductorDB() (*sql.DB, error) {
	if db_path == "" {
		return nil, fmt.Errorf("db_path is empty; call check_load_db first")
	}

	db, err := sql.Open("sqlite", db_path)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", db_path, err)
	}

	if _, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA busy_timeout = 5000;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database pragmas: %w", err)
	}

	return db, nil
}

func recordSQSMessageAndDecide(ctx context.Context, db *sql.DB, sqsMessage DatabaseSQSMessageInfo) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin database transaction: %w", err)}
	}
	defer tx.Rollback()

	parsed := ParseSQSMessageBody(sqsMessage.Body)
	message, err := upsertSQSMessage(ctx, tx, sqsMessage, parsed)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if message.AssignedAgentJobID != nil {
		if err := tx.Commit(); err != nil {
			return DatabaseCommandResult{Err: fmt.Errorf("commit existing message transaction: %w", err)}
		}
		return DatabaseCommandResult{
			Message:          message,
			DeleteSQSMessage: true,
			Reason:           "message_already_assigned_agent_job",
		}
	}

	if message.JobStatus != nil && *message.JobStatus != AgentJobStatusCreated {
		if err := tx.Commit(); err != nil {
			return DatabaseCommandResult{Err: fmt.Errorf("commit existing message status transaction: %w", err)}
		}
		return DatabaseCommandResult{
			Message:          message,
			DeleteSQSMessage: true,
			Reason:           "message_already_handled",
		}
	}

	if !shouldConsiderCloudWatchAlarmForAgentJob(message) {
		updated, err := updateMessageStatus(ctx, tx, message.ID, AgentJobStatusIgnored)
		if err != nil {
			return DatabaseCommandResult{Err: err}
		}
		if err := tx.Commit(); err != nil {
			return DatabaseCommandResult{Err: fmt.Errorf("commit ignored message transaction: %w", err)}
		}
		return DatabaseCommandResult{
			Message:          updated,
			DeleteSQSMessage: true,
			Reason:           "message_ignored",
		}
	}

	chained, err := isChainedCloudWatchAlarm(ctx, tx, message)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}
	if chained {
		updated, err := updateMessageStatus(ctx, tx, message.ID, AgentJobStatusDuplicate)
		if err != nil {
			return DatabaseCommandResult{Err: err}
		}
		if err := tx.Commit(); err != nil {
			return DatabaseCommandResult{Err: fmt.Errorf("commit duplicate message transaction: %w", err)}
		}
		return DatabaseCommandResult{
			Message:          updated,
			DeleteSQSMessage: true,
			Reason:           "cloudwatch_alarm_chained_to_existing_incident",
		}
	}

	agentJob, err := createAgentJob(ctx, tx, message.ID)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	updated, err := assignAgentJobToMessage(ctx, tx, message.ID, agentJob.ID, AgentJobStatusCreated)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit agent job transaction: %w", err)}
	}

	return DatabaseCommandResult{
		Message:             updated,
		AgentJob:            &agentJob,
		ShouldSpawnAgentJob: true,
		DeleteSQSMessage:    true,
		Reason:              "new_cloudwatch_alarm_chain",
	}
}

func shouldConsiderCloudWatchAlarmForAgentJob(message DatabaseSQSMessageInfo) bool {
	return message.MessageType == MessageTypeCloudWatchAlarm &&
		message.CloudWatchAlarmName != nil &&
		message.CloudWatchState != nil &&
		*message.CloudWatchState == "ALARM"
}

func isChainedCloudWatchAlarm(ctx context.Context, tx *sql.Tx, message DatabaseSQSMessageInfo) (bool, error) {
	if message.CloudWatchAlarmName == nil || message.EventTime == nil {
		return false, nil
	}

	periodSeconds := int64(60)
	if message.AlarmPeriodSeconds != nil && *message.AlarmPeriodSeconds > 0 {
		periodSeconds = *message.AlarmPeriodSeconds
	}
	chainWindow := time.Duration(periodSeconds*4) * time.Second

	rows, err := tx.QueryContext(ctx, `
		SELECT event_time
		FROM sqs_messages_tickets_cloudwatch
		WHERE cloudwatch_alarm_name = ?
			AND id != ?
			AND event_time IS NOT NULL
		ORDER BY event_time DESC
		LIMIT 1
	`, *message.CloudWatchAlarmName, message.ID)
	if err != nil {
		return false, fmt.Errorf("query previous cloudwatch alarm: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return false, rows.Err()
	}

	var previousEventTimeText string
	if err := rows.Scan(&previousEventTimeText); err != nil {
		return false, fmt.Errorf("scan previous cloudwatch alarm: %w", err)
	}

	previousEventTime, err := parseDatabaseTime(previousEventTimeText)
	if err != nil {
		return false, fmt.Errorf("parse previous cloudwatch event time: %w", err)
	}

	delta := message.EventTime.Sub(previousEventTime)
	if delta < 0 {
		delta = -delta
	}

	return delta <= chainWindow, rows.Err()
}

func upsertSQSMessage(ctx context.Context, tx *sql.Tx, sqsMessage DatabaseSQSMessageInfo, parsed ParsedSQSMessage) (DatabaseSQSMessageInfo, error) {
	messageType := parsed.MessageType
	if messageType == "" {
		messageType = MessageTypeUnknown
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO sqs_messages_tickets_cloudwatch (
			external_message_id,
			receipt_handle,
			external_event_id,
			raw_body,
			message_type,
			cloudwatch_alarm_name,
			cloudwatch_state,
			event_time,
			alarm_period_seconds,
			job_status,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(external_message_id) DO UPDATE SET
			receipt_handle = excluded.receipt_handle,
			updated_at = CURRENT_TIMESTAMP
	`, sqsMessage.ExternalMessageID,
		sqsMessage.ReceiptHandle,
		nullableString(parsed.ExternalEventID),
		sqsMessage.RawBody,
		messageType,
		nullableString(parsed.CloudWatchAlarmName),
		nullableString(parsed.CloudWatchState),
		nullableTime(parsed.EventTime),
		nullableInt64(parsed.AlarmPeriodSeconds),
		string(AgentJobStatusCreated),
	)
	if err != nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("insert sqs message: %w", err)
	}

	return selectSQSMessageByExternalMessageID(ctx, tx, sqsMessage.ExternalMessageID)
}

func updateMessageStatus(ctx context.Context, tx *sql.Tx, messageID int64, status AgentJobStatus) (DatabaseSQSMessageInfo, error) {
	if _, err := tx.ExecContext(ctx, `
		UPDATE sqs_messages_tickets_cloudwatch
		SET job_status = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(status), messageID); err != nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("update message status: %w", err)
	}

	return selectSQSMessageByID(ctx, tx, messageID)
}

func assignAgentJobToMessage(ctx context.Context, tx *sql.Tx, messageID int64, agentJobID int64, status AgentJobStatus) (DatabaseSQSMessageInfo, error) {
	if _, err := tx.ExecContext(ctx, `
		UPDATE sqs_messages_tickets_cloudwatch
		SET assigned_agent_job_id = ?,
			job_status = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, agentJobID, string(status), messageID); err != nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("assign agent job to message: %w", err)
	}

	return selectSQSMessageByID(ctx, tx, messageID)
}

func createAgentJob(ctx context.Context, tx *sql.Tx, messageID int64) (DatabaseAgentJobInfo, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO agent_job_info (
			agent_name,
			status,
			spawn_sqs_message_id,
			updated_at
		)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, defaultAgentJobName, string(AgentJobStatusCreated), messageID)
	if err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("create agent job: %w", err)
	}

	agentJobID, err := result.LastInsertId()
	if err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("read new agent job id: %w", err)
	}

	return selectAgentJobByID(ctx, tx, agentJobID)
}

func selectSQSMessageByExternalMessageID(ctx context.Context, tx *sql.Tx, externalMessageID string) (DatabaseSQSMessageInfo, error) {
	return scanSQSMessage(tx.QueryRowContext(ctx, selectSQSMessageSQL()+` WHERE external_message_id = ?`, externalMessageID))
}

func selectSQSMessageByID(ctx context.Context, tx *sql.Tx, messageID int64) (DatabaseSQSMessageInfo, error) {
	return scanSQSMessage(tx.QueryRowContext(ctx, selectSQSMessageSQL()+` WHERE id = ?`, messageID))
}

func selectSQSMessageSQL() string {
	return `
		SELECT
			id,
			external_message_id,
			receipt_handle,
			external_event_id,
			raw_body,
			message_type,
			cloudwatch_alarm_name,
			cloudwatch_state,
			event_time,
			alarm_period_seconds,
			assigned_agent_job_id,
			job_status,
			created_at,
			updated_at
		FROM sqs_messages_tickets_cloudwatch
	`
}

func scanSQSMessage(row *sql.Row) (DatabaseSQSMessageInfo, error) {
	var (
		message            DatabaseSQSMessageInfo
		externalEventID    sql.NullString
		cloudwatchAlarm    sql.NullString
		cloudwatchState    sql.NullString
		eventTime          sql.NullString
		alarmPeriodSeconds sql.NullInt64
		assignedAgentJobID sql.NullInt64
		jobStatus          sql.NullString
		createdAt          string
		updatedAt          string
	)

	if err := row.Scan(
		&message.ID,
		&message.ExternalMessageID,
		&message.ReceiptHandle,
		&externalEventID,
		&message.RawBody,
		&message.MessageType,
		&cloudwatchAlarm,
		&cloudwatchState,
		&eventTime,
		&alarmPeriodSeconds,
		&assignedAgentJobID,
		&jobStatus,
		&createdAt,
		&updatedAt,
	); err != nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("scan sqs message: %w", err)
	}

	message.Body = []byte(message.RawBody)
	message.ExternalEventID = stringFromNull(externalEventID)
	message.CloudWatchAlarmName = stringFromNull(cloudwatchAlarm)
	message.CloudWatchState = stringFromNull(cloudwatchState)
	message.EventTime = timeFromNull(eventTime)
	message.AlarmPeriodSeconds = int64FromNull(alarmPeriodSeconds)
	message.AssignedAgentJobID = int64FromNull(assignedAgentJobID)
	message.JobStatus = agentJobStatusFromNull(jobStatus)
	message.CreatedAt = mustParseDatabaseTime(createdAt)
	message.UpdatedAt = mustParseDatabaseTime(updatedAt)

	return message, nil
}

func selectAgentJobByID(ctx context.Context, tx *sql.Tx, agentJobID int64) (DatabaseAgentJobInfo, error) {
	var (
		agentJob       DatabaseAgentJobInfo
		status         string
		agentReport    sql.NullString
		affectedRepos  sql.NullString
		pullRequestURL sql.NullString
		failureReason  sql.NullString
		createdAt      string
		startedAt      sql.NullString
		completedAt    sql.NullString
		updatedAt      string
	)

	err := tx.QueryRowContext(ctx, `
		SELECT
			id,
			agent_name,
			status,
			spawn_sqs_message_id,
			agent_report,
			affected_repositories,
			pull_request_url,
			failure_reason,
			created_at,
			started_at,
			completed_at,
			updated_at
		FROM agent_job_info
		WHERE id = ?
	`, agentJobID).Scan(
		&agentJob.ID,
		&agentJob.AgentName,
		&status,
		&agentJob.SpawnSQSMessageID,
		&agentReport,
		&affectedRepos,
		&pullRequestURL,
		&failureReason,
		&createdAt,
		&startedAt,
		&completedAt,
		&updatedAt,
	)
	if err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("select agent job: %w", err)
	}

	agentJob.Status = AgentJobStatus(status)
	agentJob.AgentReport = stringFromNull(agentReport)
	agentJob.AffectedRepos = stringFromNull(affectedRepos)
	agentJob.PullRequestURL = stringFromNull(pullRequestURL)
	agentJob.FailureReason = stringFromNull(failureReason)
	agentJob.CreatedAt = mustParseDatabaseTime(createdAt)
	agentJob.StartedAt = timeFromNull(startedAt)
	agentJob.CompletedAt = timeFromNull(completedAt)
	agentJob.UpdatedAt = mustParseDatabaseTime(updatedAt)

	return agentJob, nil
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
