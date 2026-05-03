# Database model

The agent conductor needs a durable database record for agent work. The most
important distinction is between the message that triggered work and the agent
job created to handle that message.

## Tables

### `sqs_message`

The `sqs_message` table stores messages consumed from the shared SQS event
queue. A message may be inert, ignored, duplicated, or selected as the spawn
point for an agent job.

The table should include:

- `id`: internal primary key.
- `external_message_id`: SQS message ID.
- `external_event_id`: event ID extracted from the message body when available.
- `raw_body`: original SQS message body.
- `message_type`: coarse message category such as `cloudwatch_alarm`, `ticket`,
  or `unknown`.
- `assigned_agent_job_id`: nullable foreign key to `agent_job_info.id`.
- `job_status`: current status for this message's agent work, such as `created`,
  `running`, `succeeded`, `failed`, `ignored`, or `duplicate`.
- `created_at`: when the row was inserted.
- `updated_at`: when the row was last changed.

`assigned_agent_job_id` is only set after the conductor chooses this message as
the spawn point for an agent. If no agent should run, the column remains null.
`job_status` is stored directly on `sqs_message` so the conductor can quickly
answer "what happened to this message?" without joining through
`agent_job_info`. When `assigned_agent_job_id` is non-null, the row should point
at the corresponding `agent_job_info.id`.

### `agent_job_info`

The `agent_job_info` table stores durable state for one selected agent job. The
Go struct name should be intentionally explicit: `DatabaseAgentJobInfo`.

The table should include:

- `id`: internal primary key.
- `agent_name`: selected spawner name.
- `status`: current job state, such as `created`, `running`, `succeeded`, or
  `failed`.
- `spawn_sqs_message_id`: foreign key to the `sqs_message.id` row that caused
  this job to be created.
- `agent_report`: final or latest agent-written report.
- `affected_repositories`: text or JSON describing repos/codebases touched by
  the job.
- `pull_request_url`: PR created by the agent, when applicable.
- `failure_reason`: failure description, when applicable.
- `created_at`: when the job row was created.
- `started_at`: when the runtime wrapper started.
- `completed_at`: when the job reached a terminal state.
- `updated_at`: when the row was last changed.

## Relationship

The important relationship is:

```text
sqs_message.assigned_agent_job_id -> agent_job_info.id
agent_job_info.spawn_sqs_message_id -> sqs_message.id
```

This means the database can answer both questions:

- Given an SQS message, did it spawn an agent job?
- Given an agent job, which SQS message caused it?

## Intended flow

1. The conductor receives one SQS message.
2. The conductor inserts or updates a `sqs_message` row.
3. Registered `AgentSpawner` implementations inspect the message and return
   `AgentMatch` values.
4. If exactly one spawner is selected, the conductor creates an
   `agent_job_info` row.
5. The conductor updates `sqs_message.assigned_agent_job_id` to the new
   `agent_job_info.id` and sets `sqs_message.job_status`.
6. The conductor starts the runtime wrapper and receives `AgentEvent` updates.
7. `AgentEvent` updates mutate both `agent_job_info.status` and
   `sqs_message.job_status` until the job succeeds or fails.
