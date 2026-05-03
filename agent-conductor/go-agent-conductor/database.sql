PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS sqs_message (
    id INTEGER PRIMARY KEY,
    external_message_id TEXT NOT NULL UNIQUE,
    receipt_handle TEXT NOT NULL,
    external_event_id TEXT,
    raw_body TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'unknown',
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
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    completed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (spawn_sqs_message_id) REFERENCES sqs_message(id)
);

CREATE INDEX IF NOT EXISTS idx_sqs_message_external_event_id
    ON sqs_message(external_event_id);

CREATE INDEX IF NOT EXISTS idx_sqs_message_assigned_agent_job_id
    ON sqs_message(assigned_agent_job_id);

CREATE INDEX IF NOT EXISTS idx_sqs_message_job_status
    ON sqs_message(job_status);

CREATE INDEX IF NOT EXISTS idx_agent_job_info_status
    ON agent_job_info(status);

CREATE INDEX IF NOT EXISTS idx_agent_job_info_spawn_sqs_message_id
    ON agent_job_info(spawn_sqs_message_id);
