# Database model

The agent conductor treats SQLite as the source of truth for SQS messages,
agentJob creation decisions, and post-job debug state.

For v1, there is no separate agent-definition matching layer. The database layer
records an SQS message, parses deterministic fields from its body, inspects
current database state, and returns a conductor decision: spawn a new agentJob or
mark the message as already covered by an existing incident chain.

## Tables

### `sqs_messages_tickets_cloudwatch`

This table stores messages consumed from SQS queues that may contain tickets,
CloudWatch/EventBridge alerts, or unknown messages.

It is intentionally verbose because multiple SQS queues may exist later. This
table is the v1 decision surface for “should this message spawn an agentJob?”

Minimum columns:

- `id`: internal primary key.
- `external_message_id`: SQS message ID.
- `receipt_handle`: SQS receipt handle used to delete the message after durable
  handling.
- `external_event_id`: EventBridge/CloudWatch event ID, when present.
- `raw_body`: original SQS message body for audit/debug.
- `message_type`: coarse message category such as `cloudwatch_alarm`, `ticket`,
  or `unknown`.
- `cloudwatch_alarm_name`: parsed CloudWatch alarm name, when present.
- `cloudwatch_state`: parsed CloudWatch alarm state, such as `ALARM`, when
  present.
- `event_time`: parsed event timestamp, when present.
- `alarm_period_seconds`: parsed CloudWatch alarm period, when present.
- `assigned_agent_job_id`: nullable foreign key to `agent_job_info.id`.
- `job_status`: message-level status such as `created`, `running`, `succeeded`, `failed`, `ignored`, or `duplicate`.
- `created_at`: when the row was inserted.
- `updated_at`: when the row was last changed.

Fields that affect deterministic conductor behavior should be stored as columns.
`raw_body` is for debugging and audit, not repeated runtime decision parsing.

### `agent_job_info`

This table stores durable post-job/debug state for one spawned agentJob. The Go
struct name should remain intentionally explicit: `DatabaseAgentJobInfo`.

Minimum columns:

- `id`: internal primary key.
- `agent_name`: runtime/wrapper name. For v1 this can be a fixed Fargate Codex
  runner name, not an agent-definition match.
- `status`: current agentJob state, such as `created`, `running`, `succeeded`,
  or `failed`.
- `spawn_sqs_message_id`: foreign key to the
  `sqs_messages_tickets_cloudwatch.id` row that caused this agentJob to be
  created.
- `agent_report`: final or latest agent-written report.
- `affected_repositories`: text or JSON describing repos/codebases touched by
  the job.
- `pull_request_url`: PR created by the agent, when applicable.
- `failure_reason`: failure description, when applicable.
- `created_at`: when the row was created.
- `started_at`: when the runtime wrapper started.
- `completed_at`: when the job reached a terminal state.
- `updated_at`: when the row was last changed.

The primary “is this incident already taken?” decision should be made from
`sqs_messages_tickets_cloudwatch` state, not from `agent_job_info`.

## Relationship

The important relationship is:

```text
sqs_messages_tickets_cloudwatch.assigned_agent_job_id -> agent_job_info.id
agent_job_info.spawn_sqs_message_id -> sqs_messages_tickets_cloudwatch.id
```

This means the database can answer both questions:

- Given an SQS-derived message, did it spawn an agentJob?
- Given an agentJob, which SQS-derived message caused it?

## Intended flow

1. The conductor receives one SQS message from `poll.go`.
2. `main.go` sends a typed database command to the database worker and blocks
   waiting for its reply.
3. The database worker inserts or updates a
   `sqs_messages_tickets_cloudwatch` row.
4. `parse_message.go` parses deterministic fields from the raw body, including
   CloudWatch/EventBridge fields when present.
5. The database worker checks current rows for the same CloudWatch alarm and
   applies the time-chain rule.
6. If the message starts a new incident chain, the database worker creates an
   `agent_job_info` row and links it from
   `sqs_messages_tickets_cloudwatch.assigned_agent_job_id`.
7. If the message belongs to an existing incident chain, the database worker
   marks it as duplicate/no-spawn.
8. The database worker replies to `main.go` with the stored message row, whether
   an agentJob should be spawned, and the optional `agent_job_info` row.
9. `main.go` deletes the original SQS message only after durable database
   handling succeeds.

Actual Fargate spawning and worker event stream handling are outside this
database/sqs-message v1 implementation slice.
