package main

import (
	"context"
	"database/sql"
	"fmt"
)

type DatabaseCommandKind string

const (
	DatabaseCommandRecordSQSMessage        DatabaseCommandKind = "record_sqs_message"
	DatabaseCommandMarkAgentJobSpawned     DatabaseCommandKind = "mark_agent_job_spawned"
	DatabaseCommandMarkAgentJobSpawnFailed DatabaseCommandKind = "mark_agent_job_spawn_failed"
	DatabaseCommandQuarantineSQSMessage    DatabaseCommandKind = "quarantine_sqs_message"
	DatabaseCommandUpdateAgentJobECSStatus DatabaseCommandKind = "update_agent_job_ecs_status"
	DatabaseCommandMarkAgentJobTaskStopped DatabaseCommandKind = "mark_agent_job_task_stopped"
)

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
