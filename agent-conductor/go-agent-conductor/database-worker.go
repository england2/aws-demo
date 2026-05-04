package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"agentproto"
)

const defaultAgentJobName = "agent-fargate-codex"

type DatabaseCommandKind string

const (
	DatabaseCommandRecordSQSMessage        DatabaseCommandKind = "record_sqs_message"
	DatabaseCommandMarkAgentJobSpawned     DatabaseCommandKind = "mark_agent_job_spawned"
	DatabaseCommandMarkAgentJobSpawnFailed DatabaseCommandKind = "mark_agent_job_spawn_failed"
	DatabaseCommandRecordAgentEvent        DatabaseCommandKind = "record_agent_event"
	DatabaseCommandUpdateAgentJobECSStatus DatabaseCommandKind = "update_agent_job_ecs_status"
	DatabaseCommandMarkAgentJobTaskStopped DatabaseCommandKind = "mark_agent_job_task_stopped"
)

type DatabaseCommand struct {
	Kind              DatabaseCommandKind
	SQSMessage        DatabaseSQSMessageInfo
	AgentJobID        int64
	AgentEvent        *agentproto.AgentEvent
	AgentEventRawBody string
	ECSTaskARN        string
	ECSLastStatus     string
	ECSStoppedReason  string
	FailureReason     string
	Reply             chan DatabaseCommandResult
}

type DatabaseCommandResult struct {
	Message             DatabaseSQSMessageInfo
	AgentJob            *DatabaseAgentJobInfo
	AgentEvent          *DatabaseAgentEventInfo
	ShouldSpawnAgentJob bool
	DeleteSQSMessage    bool
	Terminal            bool
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
	command := DatabaseCommand{
		Kind:       DatabaseCommandRecordSQSMessage,
		SQSMessage: message,
	}

	return sendDatabaseCommand(ctx, commands, command)
}

func MarkAgentJobSpawned(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, taskARN string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:       DatabaseCommandMarkAgentJobSpawned,
		AgentJobID: agentJobID,
		ECSTaskARN: taskARN,
	})
}

func MarkAgentJobSpawnFailed(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, reason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:          DatabaseCommandMarkAgentJobSpawnFailed,
		AgentJobID:    agentJobID,
		FailureReason: reason,
	})
}

func RecordAgentEventWithDatabase(ctx context.Context, commands chan<- DatabaseCommand, event agentproto.AgentEvent, rawBody string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:              DatabaseCommandRecordAgentEvent,
		AgentEvent:        &event,
		AgentEventRawBody: rawBody,
	})
}

func UpdateAgentJobECSStatus(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, lastStatus string, stoppedReason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:             DatabaseCommandUpdateAgentJobECSStatus,
		AgentJobID:       agentJobID,
		ECSLastStatus:    lastStatus,
		ECSStoppedReason: stoppedReason,
	})
}

func MarkAgentJobTaskStopped(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, lastStatus string, stoppedReason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:             DatabaseCommandMarkAgentJobTaskStopped,
		AgentJobID:       agentJobID,
		ECSLastStatus:    lastStatus,
		ECSStoppedReason: stoppedReason,
		FailureReason:    stoppedReason,
	})
}

func sendDatabaseCommand(ctx context.Context, commands chan<- DatabaseCommand, command DatabaseCommand) DatabaseCommandResult {
	reply := make(chan DatabaseCommandResult, 1)
	command.Reply = reply

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
	case DatabaseCommandMarkAgentJobSpawned:
		result = markAgentJobSpawned(ctx, db, command.AgentJobID, command.ECSTaskARN)
	case DatabaseCommandMarkAgentJobSpawnFailed:
		result = markAgentJobSpawnFailed(ctx, db, command.AgentJobID, command.FailureReason)
	case DatabaseCommandRecordAgentEvent:
		result = recordAgentEvent(ctx, db, command.AgentEvent, command.AgentEventRawBody)
	case DatabaseCommandUpdateAgentJobECSStatus:
		result = updateAgentJobECSStatus(ctx, db, command.AgentJobID, command.ECSLastStatus, command.ECSStoppedReason)
	case DatabaseCommandMarkAgentJobTaskStopped:
		result = markAgentJobTaskStopped(ctx, db, command.AgentJobID, command.ECSLastStatus, command.ECSStoppedReason, command.FailureReason)
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

func markAgentJobSpawned(ctx context.Context, db *sql.DB, agentJobID int64, taskARN string) DatabaseCommandResult {
	if taskARN == "" {
		return DatabaseCommandResult{Err: fmt.Errorf("task ARN is required to mark agent job spawned")}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin spawn success transaction: %w", err)}
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET status = ?,
			ecs_task_arn = ?,
			ecs_last_status = ?,
			started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(AgentJobStatusRunning), taskARN, "PROVISIONING", agentJobID); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("mark agent job spawned: %w", err)}
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusRunning); err != nil {
		return DatabaseCommandResult{Err: err}
	}

	agentJob, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit spawn success transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: isTerminalAgentJobStatus(agentJob.Status),
		Reason:   "agent_job_spawned",
	}
}

func markAgentJobSpawnFailed(ctx context.Context, db *sql.DB, agentJobID int64, reason string) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin spawn failed transaction: %w", err)}
	}
	defer tx.Rollback()

	agentJob, err := failAgentJob(ctx, tx, agentJobID, defaultFailureReason(reason))
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit spawn failed transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: true,
		Reason:   "agent_job_spawn_failed",
	}
}

func updateAgentJobECSStatus(ctx context.Context, db *sql.DB, agentJobID int64, lastStatus string, stoppedReason string) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin ECS status transaction: %w", err)}
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET ecs_last_status = ?,
			ecs_stopped_reason = COALESCE(NULLIF(?, ''), ecs_stopped_reason),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, lastStatus, stoppedReason, agentJobID); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("update ECS status: %w", err)}
	}

	agentJob, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit ECS status transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: isTerminalAgentJobStatus(agentJob.Status),
		Reason:   "agent_job_ecs_status_updated",
	}
}

func markAgentJobTaskStopped(ctx context.Context, db *sql.DB, agentJobID int64, lastStatus string, stoppedReason string, failureReason string) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin task stopped transaction: %w", err)}
	}
	defer tx.Rollback()

	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}
	if isTerminalAgentJobStatus(current.Status) {
		if err := tx.Commit(); err != nil {
			return DatabaseCommandResult{Err: fmt.Errorf("commit already-terminal task stopped transaction: %w", err)}
		}
		return DatabaseCommandResult{
			AgentJob: &current,
			Terminal: true,
			Reason:   "agent_job_already_terminal",
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET ecs_last_status = ?,
			ecs_stopped_reason = COALESCE(NULLIF(?, ''), ecs_stopped_reason),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, lastStatus, stoppedReason, agentJobID); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("record stopped ECS task: %w", err)}
	}

	agentJob, err := failAgentJob(ctx, tx, agentJobID, defaultFailureReason(failureReason))
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit task stopped transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: true,
		Reason:   "agent_job_task_stopped_before_terminal_event",
	}
}

func recordAgentEvent(ctx context.Context, db *sql.DB, event *agentproto.AgentEvent, rawBody string) DatabaseCommandResult {
	if event == nil {
		return DatabaseCommandResult{Err: fmt.Errorf("agent event is required")}
	}
	if event.EventID == "" {
		return DatabaseCommandResult{Err: fmt.Errorf("agent event event_id is required")}
	}
	if event.JobID == "" {
		return DatabaseCommandResult{Err: fmt.Errorf("agent event job_id is required")}
	}

	agentJobID, err := strconv.ParseInt(event.JobID, 10, 64)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("parse agent event job_id %q: %w", event.JobID, err)}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin agent event transaction: %w", err)}
	}
	defer tx.Rollback()

	insertedEvent, inserted, err := insertAgentEvent(ctx, tx, agentJobID, *event, rawBody)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	agentJob, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if inserted {
		updated, err := applyAgentEventToJob(ctx, tx, agentJob, *event)
		if err != nil {
			return DatabaseCommandResult{Err: err}
		}
		agentJob = updated
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit agent event transaction: %w", err)}
	}

	reason := "agent_event_recorded"
	if !inserted {
		reason = "duplicate_agent_event"
	}

	return DatabaseCommandResult{
		AgentJob:   &agentJob,
		AgentEvent: &insertedEvent,
		Terminal:   isTerminalAgentJobStatus(agentJob.Status),
		Reason:     reason,
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

func updateLinkedMessageStatusByAgentJobID(ctx context.Context, tx *sql.Tx, agentJobID int64, status AgentJobStatus) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE sqs_messages_tickets_cloudwatch
		SET job_status = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE assigned_agent_job_id = ?
	`, string(status), agentJobID); err != nil {
		return fmt.Errorf("update linked message status: %w", err)
	}

	return nil
}

func failAgentJob(ctx context.Context, tx *sql.Tx, agentJobID int64, reason string) (DatabaseAgentJobInfo, error) {
	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}
	if isTerminalAgentJobStatus(current.Status) {
		return current, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET status = ?,
			failure_reason = ?,
			completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(AgentJobStatusFailed), reason, agentJobID); err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("mark agent job failed: %w", err)
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusFailed); err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	return selectAgentJobByID(ctx, tx, agentJobID)
}

func succeedAgentJob(ctx context.Context, tx *sql.Tx, agentJobID int64, reportPath string) (DatabaseAgentJobInfo, error) {
	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}
	if isTerminalAgentJobStatus(current.Status) {
		return current, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET status = ?,
			agent_report = COALESCE(NULLIF(?, ''), agent_report),
			completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(AgentJobStatusSucceeded), reportPath, agentJobID); err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("mark agent job succeeded: %w", err)
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusSucceeded); err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	return selectAgentJobByID(ctx, tx, agentJobID)
}

func markAgentJobRunning(ctx context.Context, tx *sql.Tx, agentJobID int64) (DatabaseAgentJobInfo, error) {
	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}
	if isTerminalAgentJobStatus(current.Status) {
		return current, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET status = ?,
			started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(AgentJobStatusRunning), agentJobID); err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("mark agent job running: %w", err)
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusRunning); err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	return selectAgentJobByID(ctx, tx, agentJobID)
}

func recordPullRequest(ctx context.Context, tx *sql.Tx, agentJobID int64, pullRequestURL string) (DatabaseAgentJobInfo, error) {
	if pullRequestURL == "" {
		return selectAgentJobByID(ctx, tx, agentJobID)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_job_info
		SET pull_request_url = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, pullRequestURL, agentJobID); err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("record pull request URL: %w", err)
	}

	return selectAgentJobByID(ctx, tx, agentJobID)
}

func applyAgentEventToJob(ctx context.Context, tx *sql.Tx, agentJob DatabaseAgentJobInfo, event agentproto.AgentEvent) (DatabaseAgentJobInfo, error) {
	switch event.Type {
	case agentproto.AgentWrapperStarted, agentproto.AgentSetupStarted, agentproto.CodexStarted:
		return markAgentJobRunning(ctx, tx, agentJob.ID)
	case agentproto.AgentSetupFailed, agentproto.AgentReportedFailure, agentproto.JobFailed:
		return failAgentJob(ctx, tx, agentJob.ID, defaultFailureReason(event.Message))
	case agentproto.JobCompleted:
		return succeedAgentJob(ctx, tx, agentJob.ID, event.ReportPath)
	case agentproto.PullRequestCreated:
		pullRequestURL := event.ArtifactURL
		if pullRequestURL == "" {
			pullRequestURL = event.Message
		}
		return recordPullRequest(ctx, tx, agentJob.ID, pullRequestURL)
	case agentproto.CodexExited, agentproto.AgentReportedSuccess:
		return selectAgentJobByID(ctx, tx, agentJob.ID)
	default:
		return selectAgentJobByID(ctx, tx, agentJob.ID)
	}
}

func insertAgentEvent(ctx context.Context, tx *sql.Tx, agentJobID int64, event agentproto.AgentEvent, rawBody string) (DatabaseAgentEventInfo, bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO agent_event (
			event_id,
			agent_job_id,
			agent_name,
			event_type,
			message,
			report_path,
			artifact_url,
			raw_body,
			created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.EventID,
		agentJobID,
		event.AgentName,
		string(event.Type),
		nullablePlainString(event.Message),
		nullablePlainString(event.ReportPath),
		nullablePlainString(event.ArtifactURL),
		rawBody,
		nullableNonZeroTime(event.CreatedAt),
	)
	if err != nil {
		return DatabaseAgentEventInfo{}, false, fmt.Errorf("insert agent event: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return DatabaseAgentEventInfo{}, false, fmt.Errorf("read inserted agent event rows affected: %w", err)
	}

	insertedEvent, err := selectAgentEventByEventID(ctx, tx, event.EventID)
	if err != nil {
		return DatabaseAgentEventInfo{}, false, err
	}

	return insertedEvent, rowsAffected > 0, nil
}

func selectAgentEventByEventID(ctx context.Context, tx *sql.Tx, eventID string) (DatabaseAgentEventInfo, error) {
	var (
		event       DatabaseAgentEventInfo
		message     sql.NullString
		reportPath  sql.NullString
		artifactURL sql.NullString
		createdAt   sql.NullString
		receivedAt  string
	)

	if err := tx.QueryRowContext(ctx, `
		SELECT
			id,
			event_id,
			agent_job_id,
			agent_name,
			event_type,
			message,
			report_path,
			artifact_url,
			raw_body,
			created_at,
			received_at
		FROM agent_event
		WHERE event_id = ?
	`, eventID).Scan(
		&event.ID,
		&event.EventID,
		&event.AgentJobID,
		&event.AgentName,
		&event.EventType,
		&message,
		&reportPath,
		&artifactURL,
		&event.RawBody,
		&createdAt,
		&receivedAt,
	); err != nil {
		return DatabaseAgentEventInfo{}, fmt.Errorf("select agent event: %w", err)
	}

	event.Message = stringFromNull(message)
	event.ReportPath = stringFromNull(reportPath)
	event.ArtifactURL = stringFromNull(artifactURL)
	event.CreatedAt = timeFromNull(createdAt)
	event.ReceivedAt = mustParseDatabaseTime(receivedAt)

	return event, nil
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
		agentJob         DatabaseAgentJobInfo
		status           string
		agentReport      sql.NullString
		affectedRepos    sql.NullString
		pullRequestURL   sql.NullString
		failureReason    sql.NullString
		ecsTaskARN       sql.NullString
		ecsLastStatus    sql.NullString
		ecsStoppedReason sql.NullString
		createdAt        string
		startedAt        sql.NullString
		completedAt      sql.NullString
		updatedAt        string
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
			ecs_task_arn,
			ecs_last_status,
			ecs_stopped_reason,
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
		&ecsTaskARN,
		&ecsLastStatus,
		&ecsStoppedReason,
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
	agentJob.ECSTaskARN = stringFromNull(ecsTaskARN)
	agentJob.ECSLastStatus = stringFromNull(ecsLastStatus)
	agentJob.ECSStoppedReason = stringFromNull(ecsStoppedReason)
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

func defaultFailureReason(reason string) string {
	if reason == "" {
		return "agent job failed"
	}

	return reason
}

func isTerminalAgentJobStatus(status AgentJobStatus) bool {
	return status == AgentJobStatusSucceeded || status == AgentJobStatusFailed
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
