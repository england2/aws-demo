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

-- name: CreateAgentJob :one
INSERT INTO agent_job_info (
    agent_name,
    status,
    spawn_sqs_message_id,
    updated_at
)
VALUES (
    sqlc.arg(agent_name),
    sqlc.arg(status),
    sqlc.arg(spawn_sqs_message_id),
    CURRENT_TIMESTAMP
)
RETURNING
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
    updated_at;

-- name: MarkAgentJobSpawned :one
UPDATE agent_job_info
SET status = sqlc.arg(status),
    ecs_task_arn = sqlc.arg(ecs_task_arn),
    ecs_last_status = sqlc.arg(ecs_last_status),
    started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
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
    updated_at;

-- name: UpdateAgentJobECSStatus :one
UPDATE agent_job_info
SET ecs_last_status = sqlc.arg(ecs_last_status),
    ecs_stopped_reason = COALESCE(NULLIF(sqlc.arg(ecs_stopped_reason), ''), ecs_stopped_reason),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
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
    updated_at;

-- name: FailAgentJob :one
UPDATE agent_job_info
SET status = sqlc.arg(status),
    failure_reason = sqlc.arg(failure_reason),
    completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
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
    updated_at;

-- name: SucceedAgentJob :one
UPDATE agent_job_info
SET status = sqlc.arg(status),
    agent_report = COALESCE(NULLIF(sqlc.arg(agent_report), ''), agent_report),
    completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
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
    updated_at;

-- name: MarkAgentJobRunning :one
UPDATE agent_job_info
SET status = sqlc.arg(status),
    started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
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
    updated_at;

-- name: RecordAgentJobPullRequest :one
UPDATE agent_job_info
SET pull_request_url = sqlc.arg(pull_request_url),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
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
    updated_at;
