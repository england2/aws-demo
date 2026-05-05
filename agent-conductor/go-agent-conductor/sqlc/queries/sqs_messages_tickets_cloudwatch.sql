-- name: GetSQSMessageByID :one
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
WHERE id = ?;

-- name: GetSQSMessageByExternalMessageID :one
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
WHERE external_message_id = ?;
