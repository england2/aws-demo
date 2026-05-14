package scheduler

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-conductor/db-internal/shared"

	_ "modernc.org/sqlite"
)

func TestRunSchedulingWithExistingAlarmFixture(t *testing.T) {
	ctx := context.Background()
	dbPath := createChainedAlarmTestDB(t)

	decisions, err := Run(ctx, Config{DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if !decisions[0].ToSchedule {
		t.Fatal("expected decision to schedule")
	}
	if decisions[0].AccountNumber != "204772699175" {
		t.Fatalf("unexpected account number: %q", decisions[0].AccountNumber)
	}
	if decisions[0].MessageType != shared.ScheduleMessageTypeIncident {
		t.Fatalf("unexpected message type: %q", decisions[0].MessageType)
	}
	if !strings.Contains(decisions[0].Text, "test-alarm-8") {
		t.Fatalf("expected newest alarm body in decision text, got %q", decisions[0].Text)
	}

	alarmCount, chainedCount, decidedCount := alarmCounts(t, dbPath)
	if alarmCount != 8 || chainedCount != 8 || decidedCount != 8 {
		t.Fatalf("unexpected alarm counts: total=%d chained=%d decided=%d", alarmCount, chainedCount, decidedCount)
	}

	secondRun, err := Run(ctx, Config{DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondRun) != 0 {
		t.Fatalf("expected no decisions on second run, got %d", len(secondRun))
	}
}

func TestRunSchedulingWithTicketMessages(t *testing.T) {
	ctx := context.Background()
	dbPath := createEmptyTestDB(t)

	worker, err := Open(ctx, Config{DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()

	err = worker.InsertTicketMessages(ctx, []IncomingMessage{
		{
			RawBody: `{
				"id": "10002",
				"key": "ENG-123",
				"fields": {
					"description": {
						"type": "doc",
						"content": [
							{
								"type": "paragraph",
								"content": [
									{"type": "text", "text": "Investigate failed login timeout."}
								]
							}
						]
					}
				}
			}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	decisions, err := worker.RunScheduling(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 ticket decision, got %d", len(decisions))
	}
	if decisions[0].MessageType != shared.ScheduleMessageTypeTicket {
		t.Fatalf("unexpected message type: %q", decisions[0].MessageType)
	}
	if decisions[0].Text != "Investigate failed login timeout." {
		t.Fatalf("unexpected ticket decision text: %q", decisions[0].Text)
	}

	secondRun, err := worker.RunScheduling(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondRun) != 0 {
		t.Fatalf("expected no ticket decisions on second run, got %d", len(secondRun))
	}
}

func createEmptyTestDB(t *testing.T) string {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("..", "db-sqlc", "database.sql"))
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "scheduler.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func createChainedAlarmTestDB(t *testing.T) string {
	t.Helper()

	dbPath := createEmptyTestDB(t)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	times := []string{
		"2026-05-12 20:00:00",
		"2026-05-12 20:12:00",
		"2026-05-12 20:24:00",
		"2026-05-12 20:36:00",
		"2026-05-12 20:48:00",
		"2026-05-12 20:59:00",
		"2026-05-12 21:11:00",
		"2026-05-12 21:23:00",
	}

	for index, receivedAt := range times {
		_, err := db.Exec(`
			INSERT INTO sqs_alarm_messages (
			    received_at,
			    raw_message_body,
			    aws_account_number
			) VALUES (?, ?, ?)
		`, receivedAt, `{"id":"test-alarm-`+string(rune('1'+index))+`"}`, "204772699175")
		if err != nil {
			t.Fatal(err)
		}
	}

	return dbPath
}

func alarmCounts(t *testing.T, dbPath string) (total int64, chained int64, decided int64) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.QueryRow(`
		SELECT count(*),
		       coalesce(sum(is_chained), 0),
		       coalesce(sum(returned_spawn_decision_for_chained_set), 0)
		FROM sqs_alarm_messages
	`).Scan(&total, &chained, &decided)
	if err != nil {
		t.Fatal(err)
	}
	return total, chained, decided
}
