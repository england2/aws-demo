package database

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

// TestProcessInboundSQSMessageCreatesThenChainsCloudWatchAlarm covers intake + chain marking.
// The first alarm creates a job; the second close-in-time alarm is marked duplicate.
func TestProcessInboundSQSMessageCreatesThenChainsCloudWatchAlarm(t *testing.T) {
	db := newTestDatabase(t)
	ctx := context.Background()

	first := processInboundSQSMessage(ctx, db, testSQSMessage("sqs-1", "event-1", "2026-05-01T04:15:00Z"))
	if first.Err != nil {
		t.Fatalf("first result error: %v", first.Err)
	}
	if !first.ShouldSpawnAgentJob {
		t.Fatalf("first ShouldSpawnAgentJob = false, reason=%s", first.Reason)
	}
	if first.AgentJob == nil {
		t.Fatalf("first AgentJob is nil")
	}
	if first.Message.JobStatus == nil || *first.Message.JobStatus != AgentJobStatusCreated {
		t.Fatalf("first JobStatus = %v, want created", first.Message.JobStatus)
	}

	second := processInboundSQSMessage(ctx, db, testSQSMessage("sqs-2", "event-2", "2026-05-01T04:16:00Z"))
	if second.Err != nil {
		t.Fatalf("second result error: %v", second.Err)
	}
	if second.ShouldSpawnAgentJob {
		t.Fatalf("second ShouldSpawnAgentJob = true, reason=%s", second.Reason)
	}
	if second.AgentJob != nil {
		t.Fatalf("second AgentJob = %+v, want nil", second.AgentJob)
	}
	if second.Message.JobStatus == nil || *second.Message.JobStatus != AgentJobStatusDuplicate {
		t.Fatalf("second JobStatus = %v, want duplicate", second.Message.JobStatus)
	}
}

// TestQuarantineSQSMessageRecordsPoisonMessage verifies poison SQS messages are durable.
// The router relies on this before deleting malformed agent-event messages from SQS.
func TestQuarantineSQSMessageRecordsPoisonMessage(t *testing.T) {
	db := newTestDatabase(t)
	ctx := context.Background()

	result := quarantineSQSMessage(ctx, db, "agent_fargate_event", "sqs-event-1", "receipt-1", `{"job_id":"manual-test"}`, `parse agent job id "manual-test"`)
	if result.Err != nil {
		t.Fatalf("quarantineSQSMessage error: %v", result.Err)
	}
	if !result.DeleteSQSMessage {
		t.Fatalf("DeleteSQSMessage = false, want true")
	}
	if result.QuarantinedMessage == nil {
		t.Fatalf("QuarantinedMessage is nil")
	}
	if result.QuarantinedMessage.QueueSource != "agent_fargate_event" {
		t.Fatalf("QueueSource = %q", result.QuarantinedMessage.QueueSource)
	}
	if result.QuarantinedMessage.QuarantineReason != `parse agent job id "manual-test"` {
		t.Fatalf("QuarantineReason = %q", result.QuarantinedMessage.QuarantineReason)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM quarantined_sqs_message WHERE external_message_id = ?`, "sqs-event-1").Scan(&count); err != nil {
		t.Fatalf("count quarantined sqs message: %v", err)
	}
	if count != 1 {
		t.Fatalf("quarantined message count = %d, want 1", count)
	}
}

// newTestDatabase creates an in-memory SQLite DB initialized with the embedded schema.
// Tests use this to exercise database transitions without touching runtime files.
func newTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(db_init); err != nil {
		t.Fatalf("init test db: %v", err)
	}

	return db
}

// testSQSMessage builds a minimal CloudWatch alarm SQS message for database intake tests.
// The body shape mirrors EventBridge alarm messages delivered through SQS.
func testSQSMessage(messageID string, eventID string, eventTime string) DatabaseSQSMessageInfo {
	body := fmt.Sprintf(`{
		"id": %q,
		"detail-type": "CloudWatch Alarm State Change",
		"source": "aws.cloudwatch",
		"time": %q,
		"detail": {
			"alarmName": "debian-cpu-spin-high-cpu",
			"state": {"value": "ALARM"},
			"configuration": {
				"metrics": [
					{"metricStat": {"period": 20}}
				]
			}
		}
	}`, eventID, eventTime)

	return DatabaseSQSMessageInfo{
		ExternalMessageID: messageID,
		ReceiptHandle:     "receipt-" + messageID,
		Body:              []byte(body),
		RawBody:           body,
	}
}
