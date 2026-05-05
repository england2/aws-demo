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
