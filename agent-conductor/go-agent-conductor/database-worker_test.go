package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRecordSQSMessageAndDecideCreatesThenChainsCloudWatchAlarm(t *testing.T) {
	db := newTestDatabase(t)
	ctx := context.Background()

	first := recordSQSMessageAndDecide(ctx, db, testSQSMessage("sqs-1", "event-1", "2026-05-01T04:15:00Z"))
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

	second := recordSQSMessageAndDecide(ctx, db, testSQSMessage("sqs-2", "event-2", "2026-05-01T04:16:00Z"))
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
