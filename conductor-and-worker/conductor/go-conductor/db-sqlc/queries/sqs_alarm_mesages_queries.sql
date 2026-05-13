-- name: InsertSQSAlarmMessage :one
INSERT INTO sqs_alarm_messages (
    raw_message_body,
    aws_account_number
) VALUES (
    ?, ?
)
RETURNING id,
          received_at,
          raw_message_body,
          aws_account_number,
          returned_spawn_decision_for_chained_set,
          is_chained;

-- name: InsertSQSAlarmMessageAt :one
INSERT INTO sqs_alarm_messages (
    received_at,
    raw_message_body,
    aws_account_number
) VALUES (
    ?, ?, ?
)
RETURNING id,
          received_at,
          raw_message_body,
          aws_account_number,
          returned_spawn_decision_for_chained_set,
          is_chained;

-- name: ListSQSAlarmMessages :many
SELECT id,
       received_at,
       raw_message_body,
       aws_account_number,
       returned_spawn_decision_for_chained_set,
       is_chained
FROM sqs_alarm_messages
ORDER BY aws_account_number ASC, received_at ASC, id ASC;

-- name: MarkSQSAlarmMessageChained :execrows
UPDATE sqs_alarm_messages
SET is_chained = 1
WHERE id = ?;

-- name: SetSQSAlarmMessageSpawnDecision :execrows
UPDATE sqs_alarm_messages
SET returned_spawn_decision_for_chained_set = ?
WHERE id = ?;

-- name: InsertSQSTicketMessage :one
INSERT INTO sqs_ticket_messages (
    raw_message_body,
    aws_account_number
) VALUES (
    ?, ?
)
RETURNING id,
          received_at,
          raw_message_body,
          aws_account_number,
          returned_spawn_decision_for_ticket;

-- name: ListSQSTicketMessagesNeedingDecision :many
SELECT id,
       received_at,
       raw_message_body,
       aws_account_number,
       returned_spawn_decision_for_ticket
FROM sqs_ticket_messages
WHERE returned_spawn_decision_for_ticket = 0
ORDER BY received_at ASC, id ASC;

-- name: SetSQSTicketMessageSpawnDecision :execrows
UPDATE sqs_ticket_messages
SET returned_spawn_decision_for_ticket = ?
WHERE id = ?;
