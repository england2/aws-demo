package database

import (
	"context"
	"database/sql"
	"fmt"

	dbgen "agent-orchestrator/internal/database/generated"
)

// quarantineSQSMessage persists malformed or unusable SQS input before deletion.
// It connects router/poller parse failures to the quarantined_sqs_message table.
// Defaults are filled here so the database row always has source, body, and reason.
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

// selectQuarantinedSQSMessageByID reads a quarantined row by primary key.
// It is a generated-query wrapper kept for consistency with other DB lookup helpers.
func selectQuarantinedSQSMessageByID(ctx context.Context, db *sql.DB, id int64) (DatabaseQuarantinedSQSMessageInfo, error) {
	message, err := dbgen.New(db).GetQuarantinedSQSMessageByID(ctx, id)
	if err != nil {
		return DatabaseQuarantinedSQSMessageInfo{}, fmt.Errorf("select quarantined sqs message: %w", err)
	}

	return databaseQuarantinedSQSMessageFromGenerated(message), nil
}
