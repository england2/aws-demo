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

-- name: GetAgentJobByID :one
SELECT
    id,
    agent_name,
    status,
    spawn_sqs_message_id,
    agent_report,
    affected_repositories,
    pull_request_url,
    failure_reason,
    ecs_task_arn,
    ecs_last_status,
    ecs_stopped_reason,
    created_at,
    started_at,
    completed_at,
    updated_at
FROM agent_job_info
WHERE id = ?;

-- name: GetAgentEventByEventID :one
SELECT
    id,
    event_id,
    agent_job_id,
    agent_name,
    event_type,
    message,
    report_path,
    artifact_url,
    raw_body,
    created_at,
    received_at
FROM agent_event
WHERE event_id = ?;

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
