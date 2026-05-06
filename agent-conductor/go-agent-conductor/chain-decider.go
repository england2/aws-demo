package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// chainDeciderMessage is the minimal row projection needed for alarm chain marking.
// It deliberately omits raw body and transport fields because chain decisions depend
// only on alarm identity, event time, period, assignment, and status.
type chainDeciderMessage struct {
	ID                 int64
	CloudWatchAlarm    string
	EventTime          time.Time
	AlarmPeriodSeconds *int64
	AssignedAgentJobID *int64
	JobStatus          *AgentJobStatus
}

// markChainedCloudWatchMessages scans persisted CloudWatch ALARM rows and marks chained rows.
// Its only job is database mutation: rows close enough to a previous alarm of the same name
// are marked duplicate so a later decider can ignore them for agent spawning.
func markChainedCloudWatchMessages(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT
			id,
			cloudwatch_alarm_name,
			event_time,
			alarm_period_seconds,
			assigned_agent_job_id,
			job_status
		FROM sqs_messages_tickets_cloudwatch
		WHERE message_type = ?
			AND cloudwatch_state = 'ALARM'
			AND cloudwatch_alarm_name IS NOT NULL
			AND event_time IS NOT NULL
		ORDER BY cloudwatch_alarm_name, event_time, id
	`, MessageTypeCloudWatchAlarm)
	if err != nil {
		return fmt.Errorf("query cloudwatch alarm chain candidates: %w", err)
	}
	defer rows.Close()

	var previous *chainDeciderMessage
	for rows.Next() {
		message, err := scanChainDeciderMessage(rows)
		if err != nil {
			return err
		}

		if shouldMarkCloudWatchMessageChained(previous, message) {
			if _, err := updateMessageStatus(ctx, tx, message.ID, AgentJobStatusDuplicate); err != nil {
				return fmt.Errorf("mark cloudwatch alarm chained: %w", err)
			}
			status := AgentJobStatusDuplicate
			message.JobStatus = &status
		}

		previous = &message
	}

	return rows.Err()
}

// scanChainDeciderMessage converts one SQL row into the chain decider projection.
// It parses DB timestamp text once so the decision loop works with time.Time values.
func scanChainDeciderMessage(rows *sql.Rows) (chainDeciderMessage, error) {
	var (
		message            chainDeciderMessage
		eventTimeText      string
		alarmPeriodSeconds sql.NullInt64
		assignedAgentJobID sql.NullInt64
		jobStatus          sql.NullString
	)

	if err := rows.Scan(
		&message.ID,
		&message.CloudWatchAlarm,
		&eventTimeText,
		&alarmPeriodSeconds,
		&assignedAgentJobID,
		&jobStatus,
	); err != nil {
		return chainDeciderMessage{}, fmt.Errorf("scan cloudwatch alarm chain candidate: %w", err)
	}

	eventTime, err := parseDatabaseTime(eventTimeText)
	if err != nil {
		return chainDeciderMessage{}, fmt.Errorf("parse cloudwatch alarm chain event time: %w", err)
	}

	message.EventTime = eventTime
	message.AlarmPeriodSeconds = int64FromNull(alarmPeriodSeconds)
	message.AssignedAgentJobID = int64FromNull(assignedAgentJobID)
	message.JobStatus = agentJobStatusFromNull(jobStatus)

	return message, nil
}

// shouldMarkCloudWatchMessageChained evaluates whether the current alarm continues a chain.
// It only marks unassigned created messages; already claimed or terminal-status messages
// must not be rewritten by a later chain pass.
func shouldMarkCloudWatchMessageChained(previous *chainDeciderMessage, message chainDeciderMessage) bool {
	if previous == nil || previous.CloudWatchAlarm != message.CloudWatchAlarm {
		return false
	}
	if message.AssignedAgentJobID != nil {
		return false
	}
	if message.JobStatus == nil || *message.JobStatus != AgentJobStatusCreated {
		return false
	}

	periodSeconds := int64(60)
	if message.AlarmPeriodSeconds != nil && *message.AlarmPeriodSeconds > 0 {
		periodSeconds = *message.AlarmPeriodSeconds
	}

	return message.EventTime.Sub(previous.EventTime) <= time.Duration(periodSeconds*4)*time.Second
}
