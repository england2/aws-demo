package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	scheduler "go-conductor/go-db-scheduler"

	_ "modernc.org/sqlite"
)

// TestSchedulerIncomingMessageFromPolledSQSMessagePreservesSchedulerFields checks the polling-to-scheduler boundary.
// It runs after ParseSQSMessageBody would have extracted the account number and confirms the scheduler receives
// the raw SQS body unchanged, because that raw body is what the database stores and schedules on.
func TestSchedulerIncomingMessageFromPolledSQSMessagePreservesSchedulerFields(t *testing.T) {
	accountNumber := "204772699175"
	polledSQSMessage := PolledSQSMessage{
		ExternalMessageID: "sqs-message-1",
		Body:              `{"source":"aws.cloudwatch"}`,
	}
	parsedSQSMessage := ParsedSQSMessage{
		AccountNumber: &accountNumber,
		MessageType:   MessageTypeCloudWatchAlarm,
	}

	schedulerIncomingMessage, err := schedulerIncomingMessageFromPolledSQSMessage(polledSQSMessage, parsedSQSMessage)
	if err != nil {
		t.Fatalf("build scheduler incoming message: %v", err)
	}

	if schedulerIncomingMessage.RawBody != polledSQSMessage.Body {
		t.Fatalf("RawBody = %q, want %q", schedulerIncomingMessage.RawBody, polledSQSMessage.Body)
	}
	if schedulerIncomingMessage.AccountNumber != accountNumber {
		t.Fatalf("AccountNumber = %q, want %q", schedulerIncomingMessage.AccountNumber, accountNumber)
	}
}

// TestSchedulerIncomingMessageFromPolledSQSMessageRequiresAccountNumber protects the scheduler insert boundary.
// It covers the path main_testing takes before opening database writes, where unsupported or incomplete SQS bodies
// should fail early instead of being shifted into a database row with guessed account metadata.
func TestSchedulerIncomingMessageFromPolledSQSMessageRequiresAccountNumber(t *testing.T) {
	_, err := schedulerIncomingMessageFromPolledSQSMessage(PolledSQSMessage{
		ExternalMessageID: "sqs-message-1",
		Body:              `{"source":"aws.cloudwatch"}`,
	}, ParsedSQSMessage{
		MessageType: MessageTypeCloudWatchAlarm,
	})
	if err == nil {
		t.Fatal("missing account number should fail")
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
	if !strings.Contains(err.Error(), "unsupported sqs message type") {
		t.Fatalf("error = %q, want unsupported message type", err)
	}
	if len(scheduleDecisions) != 0 {
		t.Fatalf("len(scheduleDecisions) = %d, want 0", len(scheduleDecisions))
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
