package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"agentproto"
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

func TestRecordAgentEventIsIdempotentAndMarksJobSucceeded(t *testing.T) {
	db := newTestDatabase(t)
	ctx := context.Background()

	decision := recordSQSMessageAndDecide(ctx, db, testSQSMessage("sqs-1", "event-1", "2026-05-01T04:15:00Z"))
	if decision.Err != nil {
		t.Fatalf("decision error: %v", decision.Err)
	}
	if decision.AgentJob == nil {
		t.Fatalf("decision AgentJob is nil")
	}

	spawned := markAgentJobSpawned(ctx, db, decision.AgentJob.ID, "arn:aws:ecs:task/test")
	if spawned.Err != nil {
		t.Fatalf("spawned error: %v", spawned.Err)
	}

	event := agentproto.AgentEvent{
		EventID:    "agent-event-1",
		JobID:      fmt.Sprint(decision.AgentJob.ID),
		AgentName:  decision.AgentJob.AgentName,
		Type:       agentproto.JobCompleted,
		ReportPath: "/tmp/report.md",
		CreatedAt:  time.Now().UTC(),
	}

	first := recordAgentEvent(ctx, db, &event, `{"event_id":"agent-event-1"}`)
	if first.Err != nil {
		t.Fatalf("first record event error: %v", first.Err)
	}
	if !first.Terminal {
		t.Fatalf("first Terminal = false, want true")
	}
	if first.AgentJob == nil || first.AgentJob.Status != AgentJobStatusSucceeded {
		t.Fatalf("first AgentJob status = %+v, want succeeded", first.AgentJob)
	}

	second := recordAgentEvent(ctx, db, &event, `{"event_id":"agent-event-1"}`)
	if second.Err != nil {
		t.Fatalf("second record event error: %v", second.Err)
	}
	if second.Reason != "duplicate_agent_event" {
		t.Fatalf("second Reason = %q, want duplicate_agent_event", second.Reason)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_event WHERE event_id = ?`, event.EventID).Scan(&count); err != nil {
		t.Fatalf("count agent event: %v", err)
	}
	if count != 1 {
		t.Fatalf("agent event count = %d, want 1", count)
	}

	var messageStatus string
	if err := db.QueryRow(`SELECT job_status FROM sqs_messages_tickets_cloudwatch WHERE id = ?`, decision.Message.ID).Scan(&messageStatus); err != nil {
		t.Fatalf("read linked message status: %v", err)
	}
	if messageStatus != string(AgentJobStatusSucceeded) {
		t.Fatalf("linked message status = %q, want succeeded", messageStatus)
	}
}

func TestDiscardAgentEventRecordsPoisonMessage(t *testing.T) {
	db := newTestDatabase(t)
	ctx := context.Background()

	result := discardAgentEvent(ctx, db, "sqs-event-1", "receipt-1", `{"job_id":"manual-test"}`, `parse agent job id "manual-test"`)
	if result.Err != nil {
		t.Fatalf("discardAgentEvent error: %v", result.Err)
	}
	if !result.DeleteSQSMessage {
		t.Fatalf("DeleteSQSMessage = false, want true")
	}
	if result.DiscardedAgentEvent == nil {
		t.Fatalf("DiscardedAgentEvent is nil")
	}
	if result.DiscardedAgentEvent.DiscardReason != `parse agent job id "manual-test"` {
		t.Fatalf("DiscardReason = %q", result.DiscardedAgentEvent.DiscardReason)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM discarded_agent_event WHERE external_message_id = ?`, "sqs-event-1").Scan(&count); err != nil {
		t.Fatalf("count discarded agent event: %v", err)
	}
	if count != 1 {
		t.Fatalf("discarded event count = %d, want 1", count)
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
