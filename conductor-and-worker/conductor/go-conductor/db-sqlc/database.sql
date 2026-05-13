CREATE TABLE IF NOT EXISTS sqs_alarm_messages (
    id INTEGER PRIMARY KEY,
    received_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    raw_message_body TEXT NOT NULL,
    aws_account_number TEXT NOT NULL,
    returned_spawn_decision_for_chained_set INTEGER NOT NULL DEFAULT 0 CHECK (returned_spawn_decision_for_chained_set IN (0, 1)),
    is_chained INTEGER NOT NULL DEFAULT 0 CHECK (is_chained IN (0, 1))
);

CREATE TABLE IF NOT EXISTS sqs_ticket_messages (
    id INTEGER PRIMARY KEY,
    received_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    raw_message_body TEXT NOT NULL,
    aws_account_number TEXT NOT NULL,
    returned_spawn_decision_for_ticket INTEGER NOT NULL DEFAULT 0 CHECK (returned_spawn_decision_for_ticket IN (0, 1))
);
