package main

import (
	"context"
	"database/sql"
	"fmt"
)

// DatabaseCommandKind names one serialized database operation handled by the worker.
// Commands keep all SQLite writes on one goroutine so state transitions stay ordered.
type DatabaseCommandKind string

const (
	DatabaseCommandProcessInboundSQSMessage DatabaseCommandKind = "process_inbound_sqs_message"
	DatabaseCommandMarkAgentJobSpawned      DatabaseCommandKind = "mark_agent_job_spawned"
	DatabaseCommandMarkAgentJobSpawnFailed  DatabaseCommandKind = "mark_agent_job_spawn_failed"
	DatabaseCommandQuarantineSQSMessage     DatabaseCommandKind = "quarantine_sqs_message"
	DatabaseCommandUpdateAgentJobECSStatus  DatabaseCommandKind = "update_agent_job_ecs_status"
	DatabaseCommandMarkAgentJobTaskStopped  DatabaseCommandKind = "mark_agent_job_task_stopped"
)

// DatabaseCommand is the request envelope sent to the single database worker.
// Different command kinds use different fields; Reply is always populated by
// sendDatabaseCommand so callers can block until durable handling completes.
type DatabaseCommand struct {
	Kind              DatabaseCommandKind
	SQSMessage        DatabaseSQSMessageInfo
	AgentJobID        int64
	RawBody           string
	QueueSource       string
	ExternalMessageID string
	ReceiptHandle     string
	QuarantineReason  string
	ECSTaskARN        string
	ECSLastStatus     string
	ECSStoppedReason  string
	FailureReason     string
	Reply             chan DatabaseCommandResult
}

// DatabaseCommandResult is the database worker response consumed by conductor loops.
// It contains refreshed DB rows plus control flags telling callers whether to spawn work,
// delete SQS messages, or stop monitoring a terminal job.
type DatabaseCommandResult struct {
	Message             DatabaseSQSMessageInfo
	AgentJob            *DatabaseAgentJobInfo
	QuarantinedMessage  *DatabaseQuarantinedSQSMessageInfo
	ShouldSpawnAgentJob bool
	DeleteSQSMessage    bool
	Terminal            bool
	Reason              string
	Err                 error
}

// StartDatabaseWorker opens the runtime DB and starts the serialized command loop.
// All conductor components send writes through this channel instead of touching SQLite
// directly, which avoids concurrent write races in the local SQLite database.
func StartDatabaseWorker(ctx context.Context) (chan<- DatabaseCommand, error) {
	db, err := openConductorDB(ctx)
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

// ProcessInboundSQSMessageWithDatabase sends one ticket/CloudWatch message to the DB worker.
// The worker records it and returns the durable decision about whether to spawn an agent job.
func ProcessInboundSQSMessageWithDatabase(ctx context.Context, commands chan<- DatabaseCommand, message DatabaseSQSMessageInfo) DatabaseCommandResult {
	command := DatabaseCommand{
		Kind:       DatabaseCommandProcessInboundSQSMessage,
		SQSMessage: message,
	}

	return sendDatabaseCommand(ctx, commands, command)
}

// MarkAgentJobSpawned records the ECS task ARN after Fargate RunTask succeeds.
// The conductor calls this before it starts monitoring agent events for that task.
func MarkAgentJobSpawned(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, taskARN string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:       DatabaseCommandMarkAgentJobSpawned,
		AgentJobID: agentJobID,
		ECSTaskARN: taskARN,
	})
}

// MarkAgentJobSpawnFailed records a failed spawn path as terminal database state.
// This preserves failures that happen after the intake transaction created a job row.
func MarkAgentJobSpawnFailed(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, reason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:          DatabaseCommandMarkAgentJobSpawnFailed,
		AgentJobID:    agentJobID,
		FailureReason: reason,
	})
}

// QuarantineSQSMessageWithDatabase stores a malformed SQS delivery before deletion.
// Router and poller code use it for poison messages that should not keep retrying forever.
func QuarantineSQSMessageWithDatabase(ctx context.Context, commands chan<- DatabaseCommand, queueSource string, externalMessageID string, receiptHandle string, rawBody string, reason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:              DatabaseCommandQuarantineSQSMessage,
		QueueSource:       queueSource,
		ExternalMessageID: externalMessageID,
		ReceiptHandle:     receiptHandle,
		RawBody:           rawBody,
		QuarantineReason:  reason,
	})
}

// UpdateAgentJobECSStatus records the latest non-terminal ECS task status.
// Running-agent monitors call this on periodic DescribeTasks observations.
func UpdateAgentJobECSStatus(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, lastStatus string, stoppedReason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:             DatabaseCommandUpdateAgentJobECSStatus,
		AgentJobID:       agentJobID,
		ECSLastStatus:    lastStatus,
		ECSStoppedReason: stoppedReason,
	})
}

// MarkAgentJobTaskStopped records an ECS STOPPED task before an agent terminal event.
// It protects the database from jobs stuck forever in running if the container crashes.
func MarkAgentJobTaskStopped(ctx context.Context, commands chan<- DatabaseCommand, agentJobID int64, lastStatus string, stoppedReason string) DatabaseCommandResult {
	return sendDatabaseCommand(ctx, commands, DatabaseCommand{
		Kind:             DatabaseCommandMarkAgentJobTaskStopped,
		AgentJobID:       agentJobID,
		ECSLastStatus:    lastStatus,
		ECSStoppedReason: stoppedReason,
		FailureReason:    stoppedReason,
	})
}

// sendDatabaseCommand sends one command and blocks until the worker replies or context ends.
// This gives callers synchronous durable handling while the actual DB writes remain serialized.
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

// handleDatabaseCommand dispatches one worker command to the concrete DB operation.
// This is the only switch that mutates SQLite, keeping database behavior centralized.
func handleDatabaseCommand(ctx context.Context, db *sql.DB, command DatabaseCommand) {
	var result DatabaseCommandResult

	switch command.Kind {
	case DatabaseCommandProcessInboundSQSMessage:
		result = processInboundSQSMessage(ctx, db, command.SQSMessage)
	case DatabaseCommandMarkAgentJobSpawned:
		result = markAgentJobSpawned(ctx, db, command.AgentJobID, command.ECSTaskARN)
	case DatabaseCommandMarkAgentJobSpawnFailed:
		result = markAgentJobSpawnFailed(ctx, db, command.AgentJobID, command.FailureReason)
	case DatabaseCommandQuarantineSQSMessage:
		result = quarantineSQSMessage(ctx, db, command.QueueSource, command.ExternalMessageID, command.ReceiptHandle, command.RawBody, command.QuarantineReason)
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
