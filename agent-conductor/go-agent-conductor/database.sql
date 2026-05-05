PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS sqs_messages_tickets_cloudwatch (
    id INTEGER PRIMARY KEY,
    external_message_id TEXT NOT NULL UNIQUE,
    receipt_handle TEXT NOT NULL,
    external_event_id TEXT,
    raw_body TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'unknown',
    cloudwatch_alarm_name TEXT,
    cloudwatch_state TEXT,
    event_time TEXT,
    alarm_period_seconds INTEGER,
    assigned_agent_job_id INTEGER,
    job_status TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (assigned_agent_job_id) REFERENCES agent_job_info(id)
);

CREATE TABLE IF NOT EXISTS agent_job_info (
    id INTEGER PRIMARY KEY,
    agent_name TEXT NOT NULL,
    status TEXT NOT NULL,
    spawn_sqs_message_id INTEGER NOT NULL,
    agent_report TEXT,
    affected_repositories TEXT,
    pull_request_url TEXT,
    failure_reason TEXT,
    ecs_task_arn TEXT,
    ecs_last_status TEXT,
    ecs_stopped_reason TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    completed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (spawn_sqs_message_id) REFERENCES sqs_messages_tickets_cloudwatch(id)
);

CREATE INDEX IF NOT EXISTS idx_sqs_messages_tickets_cloudwatch_external_event_id
    ON sqs_messages_tickets_cloudwatch(external_event_id);

CREATE INDEX IF NOT EXISTS idx_sqs_messages_tickets_cloudwatch_assigned_agent_job_id
    ON sqs_messages_tickets_cloudwatch(assigned_agent_job_id);

CREATE INDEX IF NOT EXISTS idx_sqs_messages_tickets_cloudwatch_job_status
    ON sqs_messages_tickets_cloudwatch(job_status);

CREATE INDEX IF NOT EXISTS idx_sqs_messages_tickets_cloudwatch_alarm_name_event_time
    ON sqs_messages_tickets_cloudwatch(cloudwatch_alarm_name, event_time);

CREATE INDEX IF NOT EXISTS idx_agent_job_info_status
    ON agent_job_info(status);

CREATE INDEX IF NOT EXISTS idx_agent_job_info_spawn_sqs_message_id
    ON agent_job_info(spawn_sqs_message_id);

CREATE TABLE IF NOT EXISTS agent_event (
    id INTEGER PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    agent_job_id INTEGER NOT NULL,
    agent_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    message TEXT,
    report_path TEXT,
    artifact_url TEXT,
    raw_body TEXT NOT NULL,
    created_at TEXT,
    received_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_job_id) REFERENCES agent_job_info(id)
);

CREATE INDEX IF NOT EXISTS idx_agent_event_agent_job_id
    ON agent_event(agent_job_id);

CREATE INDEX IF NOT EXISTS idx_agent_event_event_type
    ON agent_event(event_type);

CREATE TABLE IF NOT EXISTS discarded_agent_event (
    id INTEGER PRIMARY KEY,
    external_message_id TEXT,
    receipt_handle TEXT,
    raw_body TEXT NOT NULL,
    discard_reason TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
