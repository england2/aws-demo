-- name: GetQuarantinedSQSMessageByID :one
SELECT
    id,
    queue_source,
    external_message_id,
    receipt_handle,
    raw_body,
    quarantine_reason,
    created_at
FROM quarantined_sqs_message
WHERE id = ?;

-- name: CreateQuarantinedSQSMessage :one
INSERT INTO quarantined_sqs_message (
    queue_source,
    external_message_id,
    receipt_handle,
    raw_body,
    quarantine_reason
)
VALUES (
    sqlc.arg(queue_source),
    sqlc.arg(external_message_id),
    sqlc.arg(receipt_handle),
    sqlc.arg(raw_body),
    sqlc.arg(quarantine_reason)
)
RETURNING
    id,
    queue_source,
    external_message_id,
    receipt_handle,
    raw_body,
    quarantine_reason,
    created_at;
