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
