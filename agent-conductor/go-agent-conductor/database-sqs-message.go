package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbgen "agent-orchestrator/internal/db/generated"
)

func processInboundSQSMessage(ctx context.Context, db *sql.DB, sqsMessage DatabaseSQSMessageInfo) DatabaseCommandResult {
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

func selectSQSMessageByExternalMessageID(ctx context.Context, tx *sql.Tx, externalMessageID string) (DatabaseSQSMessageInfo, error) {
	message, err := dbgen.New(tx).GetSQSMessageByExternalMessageID(ctx, externalMessageID)
	if err != nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("select sqs message: %w", err)
	}

	return databaseSQSMessageFromGenerated(message), nil
}

func selectSQSMessageByID(ctx context.Context, tx *sql.Tx, messageID int64) (DatabaseSQSMessageInfo, error) {
	message, err := dbgen.New(tx).GetSQSMessageByID(ctx, messageID)
	if err != nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("select sqs message: %w", err)
	}

	return databaseSQSMessageFromGenerated(message), nil
}
