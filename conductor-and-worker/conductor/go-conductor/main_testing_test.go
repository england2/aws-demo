package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-conductor/db-internal/shared"
	scheduler "go-conductor/go-db-scheduler"

	_ "modernc.org/sqlite"
)

// TestSchedulerIncomingMessageFromPolledSQSMessagePreservesRawBody checks the polling-to-scheduler boundary.
// It confirms main only adapts the transport envelope and leaves incident/ticket classification to the scheduler
// package before any database insert or scheduling decision is made.
func TestSchedulerIncomingMessageFromPolledSQSMessagePreservesRawBody(t *testing.T) {
	polledSQSMessage := PolledSQSMessage{
		ExternalMessageID: "sqs-message-1",
		Body:              `{"source":"aws.cloudwatch"}`,
	}

	schedulerIncomingMessage := schedulerIncomingMessageFromPolledSQSMessage(polledSQSMessage)

	if schedulerIncomingMessage.RawBody != polledSQSMessage.Body {
		t.Fatalf("RawBody = %q, want %q", schedulerIncomingMessage.RawBody, polledSQSMessage.Body)
	}
	if schedulerIncomingMessage.AccountNumber != "" {
		t.Fatalf("AccountNumber = %q, want empty scheduler-owned classification field", schedulerIncomingMessage.AccountNumber)
	}
}

// counterfactual confirmed
// TestConductorWorkerDialAddressUsesExplicitWorkerDialAddress covers the AWS deployment address split.
// The conductor listens on all interfaces inside Docker, while spawned Fargate workers receive the private EC2
// address that reaches the host-published gRPC port.
func TestConductorWorkerDialAddressUsesExplicitWorkerDialAddress(t *testing.T) {
	originalServerAddr := *serverAddr
	originalWorkerDialAddr := *workerDialAddr
	t.Cleanup(func() {
		*serverAddr = originalServerAddr
		*workerDialAddr = originalWorkerDialAddr
	})

	*serverAddr = "0.0.0.0:50055"
	*workerDialAddr = "172.31.14.8:50055"

	if got := conductorWorkerDialAddress(); got != "172.31.14.8:50055" {
		t.Fatalf("conductorWorkerDialAddress() = %q, want private worker dial address", got)
	}
}

// counterfactual confirmed
// TestConductorWorkerDialAddressFallsBackToListenAddress preserves the local smoke-test path.
// Local Docker workers still use the existing addr flag when no AWS-specific worker dial address has been
// configured by deployment.
func TestConductorWorkerDialAddressFallsBackToListenAddress(t *testing.T) {
	originalServerAddr := *serverAddr
	originalWorkerDialAddr := *workerDialAddr
	t.Cleanup(func() {
		*serverAddr = originalServerAddr
		*workerDialAddr = originalWorkerDialAddr
	})

	*serverAddr = "localhost:50055"
	*workerDialAddr = ""

	if got := conductorWorkerDialAddress(); got != "localhost:50055" {
		t.Fatalf("conductorWorkerDialAddress() = %q, want listen address fallback", got)
	}
}

// TestInsertPolledSQSMessageAndRunSchedulerPersistsSchedulesAndReturnsDecisions exercises the scheduler handoff.
// It starts with one existing alarm row, inserts the polled CloudWatch message, runs scheduler chaining, and returns
// decisions to the caller so main_testing can own the later SQS delete.
func TestInsertPolledSQSMessageAndRunSchedulerPersistsSchedulesAndReturnsDecisions(t *testing.T) {
	ctx := context.Background()
	schedulerDatabasePath := createEmptyMainTestingSchedulerDatabase(t)
	insertPreviousAlarmMessage(t, schedulerDatabasePath)

	schedulerWorker, err := scheduler.Open(ctx, scheduler.Config{DBPath: schedulerDatabasePath})
	if err != nil {
		t.Fatalf("open scheduler: %v", err)
	}
	defer schedulerWorker.Close()

	scheduleDecisions, err := insertPolledSQSMessageAndRunScheduler(ctx, schedulerWorker, PolledSQSMessage{
		ExternalMessageID: "sqs-message-2",
		ReceiptHandle:     "receipt-handle-2",
		Body: `{
			"id": "event-2",
			"account": "204772699175",
			"detail-type": "CloudWatch Alarm State Change",
			"source": "aws.cloudwatch",
			"time": "2026-05-01T04:16:44Z",
			"detail": {
				"alarmName": "debian-cpu-spin-high-cpu",
				"state": {"value": "ALARM"},
				"configuration": {
					"metrics": [
						{"metricStat": {"period": 20}}
					]
				}
			}
		}`,
	})
	if err != nil {
		t.Fatalf("insert polled sqs message and run scheduler: %v", err)
	}
	if len(scheduleDecisions) != 1 {
		t.Fatalf("len(scheduleDecisions) = %d, want 1", len(scheduleDecisions))
	}
	if !scheduleDecisions[0].ToSchedule {
		t.Fatal("expected scheduler decision to schedule")
	}
	if scheduleDecisions[0].AccountNumber != "204772699175" {
		t.Fatalf("AccountNumber = %q, want 204772699175", scheduleDecisions[0].AccountNumber)
	}
	if scheduleDecisions[0].MessageType != shared.ScheduleMessageTypeIncident {
		t.Fatalf("MessageType = %q, want %q", scheduleDecisions[0].MessageType, shared.ScheduleMessageTypeIncident)
	}

	totalAlarmRows, chainedAlarmRows, decidedAlarmRows := alarmMessageCounts(t, schedulerDatabasePath)
	if totalAlarmRows != 2 || chainedAlarmRows != 2 || decidedAlarmRows != 2 {
		t.Fatalf("alarm counts total=%d chained=%d decided=%d, want 2/2/2", totalAlarmRows, chainedAlarmRows, decidedAlarmRows)
	}
}

// TestInsertPolledSQSMessageAndRunSchedulerRejectsUnsupportedBeforeDelete covers deterministic unsupported input.
// It verifies unsupported messages fail before being forced through the scheduler's database shape as though account
// metadata had been established, leaving main_testing with an error and no schedule decisions.
func TestInsertPolledSQSMessageAndRunSchedulerRejectsUnsupportedBeforeDelete(t *testing.T) {
	ctx := context.Background()
	schedulerDatabasePath := createEmptyMainTestingSchedulerDatabase(t)

	schedulerWorker, err := scheduler.Open(ctx, scheduler.Config{DBPath: schedulerDatabasePath})
	if err != nil {
		t.Fatalf("open scheduler: %v", err)
	}
	defer schedulerWorker.Close()

	scheduleDecisions, err := insertPolledSQSMessageAndRunScheduler(ctx, schedulerWorker, PolledSQSMessage{
		ExternalMessageID: "sqs-message-unsupported",
		ReceiptHandle:     "receipt-handle-unsupported",
		Body:              `{"account":"204772699175","source":"not-cloudwatch"}`,
	})
	if err == nil {
		t.Fatal("unsupported sqs message should fail")
	}
	if !strings.Contains(err.Error(), "unsupported scheduler message shape") {
		t.Fatalf("error = %q, want unsupported scheduler message shape", err)
	}
	if len(scheduleDecisions) != 0 {
		t.Fatalf("len(scheduleDecisions) = %d, want 0", len(scheduleDecisions))
	}
}

// counterfactual confirmed
// TestInsertPolledSQSMessageAndRunSchedulerSchedulesTicketDescription covers the ticket polling path.
// It feeds the documented Jira-style shape through main's scheduler handoff and verifies the returned decision
// carries ticket type plus flattened description text for later prompt selection.
func TestInsertPolledSQSMessageAndRunSchedulerSchedulesTicketDescription(t *testing.T) {
	ctx := context.Background()
	schedulerDatabasePath := createEmptyMainTestingSchedulerDatabase(t)

	schedulerWorker, err := scheduler.Open(ctx, scheduler.Config{DBPath: schedulerDatabasePath})
	if err != nil {
		t.Fatalf("open scheduler: %v", err)
	}
	defer schedulerWorker.Close()

	scheduleDecisions, err := insertPolledSQSMessageAndRunScheduler(ctx, schedulerWorker, PolledSQSMessage{
		ExternalMessageID: "sqs-ticket-1",
		ReceiptHandle:     "receipt-handle-ticket-1",
		Body: `{
			"id": "10002",
			"key": "ENG-123",
			"fields": {
				"description": {
					"type": "doc",
					"version": 1,
					"content": [
						{
							"type": "paragraph",
							"content": [
								{
									"type": "text",
									"text": "Users are redirected to a blank page after their session expires."
								}
							]
						}
					]
				}
			}
		}`,
	})
	if err != nil {
		t.Fatalf("insert ticket sqs message and run scheduler: %v", err)
	}
	if len(scheduleDecisions) != 1 {
		t.Fatalf("len(scheduleDecisions) = %d, want 1", len(scheduleDecisions))
	}
	if scheduleDecisions[0].MessageType != shared.ScheduleMessageTypeTicket {
		t.Fatalf("MessageType = %q, want %q", scheduleDecisions[0].MessageType, shared.ScheduleMessageTypeTicket)
	}
	if scheduleDecisions[0].Text != "Users are redirected to a blank page after their session expires." {
		t.Fatalf("Text = %q", scheduleDecisions[0].Text)
	}
}

// TestSchedulerDatabasePathFromFlagValuesSupportsPositionalFlagName verifies the local run command shape.
// It covers `conductor -- test-db-loc /path/to/db.sqlite`, where Go flag parsing leaves the name and value as
// positional args and the conductor must choose the second arg as the actual SQLite path.
func TestSchedulerDatabasePathFromFlagValuesSupportsPositionalFlagName(t *testing.T) {
	schedulerDatabasePath := schedulerDatabasePathFromFlagValues("", []string{
		"test-db-loc",
		"/tmp/test-database.sqlite",
	})

	if schedulerDatabasePath != "/tmp/test-database.sqlite" {
		t.Fatalf("schedulerDatabasePath = %q, want /tmp/test-database.sqlite", schedulerDatabasePath)
	}
}

// TestSchedulerDatabasePathFromFlagValuesPrefersParsedFlag keeps normal Go flag usage ahead of positional args.
// It runs before any database compliance check, ensuring a parsed -test-db-loc value is not displaced by leftovers
// after a "--" separator.
func TestSchedulerDatabasePathFromFlagValuesPrefersParsedFlag(t *testing.T) {
	schedulerDatabasePath := schedulerDatabasePathFromFlagValues("/tmp/from-flag.sqlite", []string{
		"test-db-loc",
		"/tmp/from-args.sqlite",
	})

	if schedulerDatabasePath != "/tmp/from-flag.sqlite" {
		t.Fatalf("schedulerDatabasePath = %q, want /tmp/from-flag.sqlite", schedulerDatabasePath)
	}
}

func createEmptyMainTestingSchedulerDatabase(t *testing.T) string {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("db-sqlc", "database.sql"))
	if err != nil {
		t.Fatalf("read scheduler schema: %v", err)
	}

	schedulerDatabasePath := filepath.Join(t.TempDir(), "scheduler.sqlite")
	database, err := sql.Open("sqlite", schedulerDatabasePath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return schedulerDatabasePath
}

func insertPreviousAlarmMessage(t *testing.T, schedulerDatabasePath string) {
	t.Helper()

	database, err := sql.Open("sqlite", schedulerDatabasePath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	receivedAt := time.Now().UTC().Add(-5 * time.Minute).Format("2006-01-02 15:04:05")
	_, err = database.Exec(`
		INSERT INTO sqs_alarm_messages (
		    received_at,
		    raw_message_body,
		    aws_account_number
		) VALUES (?, ?, ?)
	`, receivedAt, `{"id":"event-1"}`, "204772699175")
	if err != nil {
		t.Fatalf("insert previous alarm: %v", err)
	}
}

func alarmMessageCounts(t *testing.T, schedulerDatabasePath string) (total int64, chained int64, decided int64) {
	t.Helper()

	database, err := sql.Open("sqlite", schedulerDatabasePath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	err = database.QueryRow(`
		SELECT count(*),
		       coalesce(sum(is_chained), 0),
		       coalesce(sum(returned_spawn_decision_for_chained_set), 0)
		FROM sqs_alarm_messages
	`).Scan(&total, &chained, &decided)
	if err != nil {
		t.Fatalf("query alarm counts: %v", err)
	}
	return total, chained, decided
}
