package main

import (
	"context"
	"database/sql"
	"fmt"

	dbgen "agent-orchestrator/internal/db/generated"
)


func quarantineSQSMessage(ctx context.Context, db *sql.DB, queueSource string, externalMessageID string, receiptHandle string, rawBody string, reason string) DatabaseCommandResult {
	if queueSource == "" {
		queueSource = "unknown"
	}
	if rawBody == "" {
		rawBody = "{}"
	}
	if reason == "" {
		reason = "unspecified"
	}

	result, err := db.ExecContext(ctx, `
		INSERT INTO quarantined_sqs_message (
			queue_source,
			external_message_id,
			receipt_handle,
			raw_body,
			quarantine_reason
		)
		VALUES (?, ?, ?, ?, ?)
	`, queueSource, nullablePlainString(externalMessageID), nullablePlainString(receiptHandle), rawBody, reason)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("insert quarantined sqs message: %w", err)}
	}

	id, err := result.LastInsertId()
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("read quarantined sqs message id: %w", err)}
	}

	quarantined, err := selectQuarantinedSQSMessageByID(ctx, db, id)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	return DatabaseCommandResult{
		QuarantinedMessage: &quarantined,
		DeleteSQSMessage:   true,
		Reason:             "quarantined_sqs_message",
	}
}

func selectQuarantinedSQSMessageByID(ctx context.Context, db *sql.DB, id int64) (DatabaseQuarantinedSQSMessageInfo, error) {
	message, err := dbgen.New(db).GetQuarantinedSQSMessageByID(ctx, id)
	if err != nil {
		return DatabaseQuarantinedSQSMessageInfo{}, fmt.Errorf("select quarantined sqs message: %w", err)
	}

	return databaseQuarantinedSQSMessageFromGenerated(message), nil
}
