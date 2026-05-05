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

	row, err := dbgen.New(db).CreateQuarantinedSQSMessage(ctx, dbgen.CreateQuarantinedSQSMessageParams{
		QueueSource:       queueSource,
		ExternalMessageID: sqlNullString(externalMessageID),
		ReceiptHandle:     sqlNullString(receiptHandle),
		RawBody:           rawBody,
		QuarantineReason:  reason,
	})
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("insert quarantined sqs message: %w", err)}
	}

	quarantined := databaseQuarantinedSQSMessageFromGenerated(row)

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
