package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"go-conductor/db-internal/shared"
	sqlcgen "go-conductor/db-internal/sqlc-generated"

	_ "modernc.org/sqlite"
)

const chainWindow = time.Hour

var schedulerDebugLogger = slog.New(newSchedulerStdoutHandler(os.Stdout))

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

type schedulerStdoutHandler struct {
	writer io.Writer
	mutex  *sync.Mutex
	attrs  []slog.Attr
}

// newSchedulerStdoutHandler keeps slog as the scheduler logging API while printing compact smoke-test lines.
// The output shape is intentionally human-first: "[SHEDULER timestamp] message key=value" on stdout.
func newSchedulerStdoutHandler(writer io.Writer) slog.Handler {
	return &schedulerStdoutHandler{
		writer: writer,
		mutex:  &sync.Mutex{},
	}
}

func (handler *schedulerStdoutHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (handler *schedulerStdoutHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	timestamp := record.Time.Format("2006-01-02--15:04:05")
	line.WriteString("[SHEDULER ")
	line.WriteString(timestamp)
	line.WriteString("] ")
	line.WriteString(record.Message)

	writeAttr := func(attr slog.Attr) {
		attr.Value = attr.Value.Resolve()
		if attr.Equal(slog.Attr{}) {
			return
		}
		line.WriteByte(' ')
		line.WriteString(attr.Key)
		line.WriteByte('=')
		line.WriteString(attr.Value.String())
	}
	for _, attr := range handler.attrs {
		writeAttr(attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		writeAttr(attr)
		return true
	})
	line.WriteByte('\n')

	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	_, err := io.WriteString(handler.writer, line.String())
	return err
}

func (handler *schedulerStdoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copiedAttrs := make([]slog.Attr, 0, len(handler.attrs)+len(attrs))
	copiedAttrs = append(copiedAttrs, handler.attrs...)
	copiedAttrs = append(copiedAttrs, attrs...)
	return &schedulerStdoutHandler{
		writer: handler.writer,
		mutex:  handler.mutex,
		attrs:  copiedAttrs,
	}
}

func (handler *schedulerStdoutHandler) WithGroup(string) slog.Handler {
	return handler
}

// logSchedulerDebug prints timestamped scheduler trace lines for local conductor smoke runs.
// It wraps slog so the scheduler gets stdout now and keeps a structured logging path for future OTEL bridging.
func logSchedulerDebug(message string, args ...any) {
	schedulerDebugLogger.Debug(message, args...)
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

// InsertAlarmMessageAndRunScheduling persists one alarm message and immediately runs the scheduler transaction.
// Conductor polling calls this after it has translated an SQS delivery into IncomingMessage; the returned decisions
// are handed back to main so external SQS delete ordering stays outside the scheduler package.
func (w *Worker) InsertAlarmMessageAndRunScheduling(ctx context.Context, message IncomingMessage) ([]shared.ScheduleDecision, error) {
	logSchedulerDebug("inserting alarm message", "account", message.AccountNumber, "body_bytes", len(message.RawBody))
	if err := w.InsertAlarmMessages(ctx, []IncomingMessage{message}); err != nil {
		return nil, err
	}

	return w.RunScheduling(ctx)
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
	logSchedulerDebug("starting scheduling pass")

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

	logSchedulerDebug("scheduling pass complete", "decisions", len(decisions))
	return decisions, nil
}

func scheduleAlarms(ctx context.Context, q *sqlcgen.Queries) ([]shared.ScheduleDecision, error) {
	rows, err := q.ListSQSAlarmMessages(ctx)
	if err != nil {
		return nil, err
	}
	logSchedulerDebug("loaded alarm rows", "count", len(rows))
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
		if len(chain) > 0 {
			logSchedulerDebug("evaluating alarm chain", "account", chain[0].AwsAccountNumber, "rows", len(chain))
		}
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
		logSchedulerDebug("no alarm decision: chain has fewer than 2 rows")
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
		logSchedulerDebug("no alarm decision: chain already returned a spawn decision")
		return shared.ScheduleDecision{}, false, nil
	}

	newest := chain[len(chain)-1]
	logSchedulerDebug("scheduling alarm decision", "account", newest.AwsAccountNumber, "chain_rows", len(chain), "newest_alarm_row_id", newest.ID)
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
	logSchedulerDebug("loaded ticket rows needing decision", "count", len(rows))

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
		logSchedulerDebug("scheduling ticket decision", "account", row.AwsAccountNumber, "ticket_row_id", row.ID)
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
