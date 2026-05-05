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
