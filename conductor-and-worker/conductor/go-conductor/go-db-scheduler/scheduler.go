package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"go-conductor/db-internal/shared"
	sqlcgen "go-conductor/db-internal/sqlc-generated"

	_ "modernc.org/sqlite"
)

const chainWindow = time.Hour

type Config struct {
	DBPath string
}

type IncomingMessage struct {
	RawBody       string
	AccountNumber string
}

type Worker struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

func Run(ctx context.Context, cfg Config) ([]shared.ScheduleDecision, error) {
	worker, err := Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer worker.Close()

	return worker.RunScheduling(ctx)
}

func Open(ctx context.Context, cfg Config) (*Worker, error) {
	if cfg.DBPath == "" {
		return nil, errors.New("scheduler config DBPath is required")
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return &Worker{
		db: db,
		q:  sqlcgen.New(db),
	}, nil
}

func (w *Worker) Close() error {
	if w == nil || w.db == nil {
		return nil
	}
	return w.db.Close()
}

func (w *Worker) InsertAlarmMessages(ctx context.Context, messages []IncomingMessage) error {
	for _, message := range messages {
		if _, err := w.q.InsertSQSAlarmMessage(ctx, sqlcgen.InsertSQSAlarmMessageParams{
			RawMessageBody:   message.RawBody,
			AwsAccountNumber: message.AccountNumber,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) InsertTicketMessages(ctx context.Context, messages []IncomingMessage) error {
	for _, message := range messages {
		if _, err := w.q.InsertSQSTicketMessage(ctx, sqlcgen.InsertSQSTicketMessageParams{
			RawMessageBody:   message.RawBody,
			AwsAccountNumber: message.AccountNumber,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) RunScheduling(ctx context.Context) ([]shared.ScheduleDecision, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	qtx := w.q.WithTx(tx)

	alarmDecisions, err := scheduleAlarms(ctx, qtx)
	if err != nil {
		return nil, err
	}
	ticketDecisions, err := scheduleTickets(ctx, qtx)
	if err != nil {
		return nil, err
	}

	decisions := make([]shared.ScheduleDecision, 0, len(alarmDecisions)+len(ticketDecisions))
	decisions = append(decisions, alarmDecisions...)
	decisions = append(decisions, ticketDecisions...)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return decisions, nil
}

func scheduleAlarms(ctx context.Context, q *sqlcgen.Queries) ([]shared.ScheduleDecision, error) {
	rows, err := q.ListSQSAlarmMessages(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].AwsAccountNumber != rows[j].AwsAccountNumber {
			return rows[i].AwsAccountNumber < rows[j].AwsAccountNumber
		}
		if rows[i].ReceivedAt != rows[j].ReceivedAt {
			return rows[i].ReceivedAt < rows[j].ReceivedAt
		}
		return rows[i].ID < rows[j].ID
	})

	var decisions []shared.ScheduleDecision
	var current []sqlcgen.SqsAlarmMessage

	flush := func(chain []sqlcgen.SqsAlarmMessage) error {
		decision, ok, err := processAlarmChain(ctx, q, chain)
		if err != nil {
			return err
		}
		if ok {
			decisions = append(decisions, decision)
		}
		return nil
	}

	for _, row := range rows {
		if len(current) == 0 {
			current = append(current, row)
			continue
		}

		previous := current[len(current)-1]
		chained, err := alarmRowsAreChained(previous, row)
		if err != nil {
			return nil, err
		}

		if previous.AwsAccountNumber == row.AwsAccountNumber && chained {
			current = append(current, row)
			continue
		}

		if err := flush(current); err != nil {
			return nil, err
		}
		current = []sqlcgen.SqsAlarmMessage{row}
	}

	if err := flush(current); err != nil {
		return nil, err
	}

	return decisions, nil
}

func processAlarmChain(ctx context.Context, q *sqlcgen.Queries, chain []sqlcgen.SqsAlarmMessage) (shared.ScheduleDecision, bool, error) {
	if len(chain) < 2 {
		return shared.ScheduleDecision{}, false, nil
	}

	alreadyDecided := false
	for _, row := range chain {
		if row.IsChained == 0 {
			if _, err := q.MarkSQSAlarmMessageChained(ctx, row.ID); err != nil {
				return shared.ScheduleDecision{}, false, err
			}
		}
		if row.ReturnedSpawnDecisionForChainedSet != 0 {
			alreadyDecided = true
		}
	}

	for _, row := range chain {
		if row.ReturnedSpawnDecisionForChainedSet == 0 {
			if _, err := q.SetSQSAlarmMessageSpawnDecision(ctx, sqlcgen.SetSQSAlarmMessageSpawnDecisionParams{
				ReturnedSpawnDecisionForChainedSet: 1,
				ID:                                 row.ID,
			}); err != nil {
				return shared.ScheduleDecision{}, false, err
			}
		}
	}

	if alreadyDecided {
		return shared.ScheduleDecision{}, false, nil
	}

	newest := chain[len(chain)-1]
	return shared.ScheduleDecision{
		ToSchedule:    true,
		Text:          newest.RawMessageBody,
		AccountNumber: newest.AwsAccountNumber,
	}, true, nil
}

func scheduleTickets(ctx context.Context, q *sqlcgen.Queries) ([]shared.ScheduleDecision, error) {
	rows, err := q.ListSQSTicketMessagesNeedingDecision(ctx)
	if err != nil {
		return nil, err
	}

	decisions := make([]shared.ScheduleDecision, 0, len(rows))
	for _, row := range rows {
		if _, err := q.SetSQSTicketMessageSpawnDecision(ctx, sqlcgen.SetSQSTicketMessageSpawnDecisionParams{
			ReturnedSpawnDecisionForTicket: 1,
			ID:                             row.ID,
		}); err != nil {
			return nil, err
		}

		decisions = append(decisions, shared.ScheduleDecision{
			ToSchedule:    true,
			Text:          row.RawMessageBody,
			AccountNumber: row.AwsAccountNumber,
		})
	}

	return decisions, nil
}

func alarmRowsAreChained(previous, next sqlcgen.SqsAlarmMessage) (bool, error) {
	previousTime, err := parseReceivedAt(previous.ReceivedAt)
	if err != nil {
		return false, fmt.Errorf("parse previous alarm %d received_at %q: %w", previous.ID, previous.ReceivedAt, err)
	}
	nextTime, err := parseReceivedAt(next.ReceivedAt)
	if err != nil {
		return false, fmt.Errorf("parse next alarm %d received_at %q: %w", next.ID, next.ReceivedAt, err)
	}

	return !nextTime.Before(previousTime) && nextTime.Sub(previousTime) <= chainWindow, nil
}

func parseReceivedAt(value string) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, value)
}
