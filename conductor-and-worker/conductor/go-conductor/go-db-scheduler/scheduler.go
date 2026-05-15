package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
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

type parsedIncomingMessage struct {
	MessageType   shared.ScheduleMessageType
	Text          string
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

// InsertMessageAndRunScheduling classifies one raw inbound SQS message, stores it in the matching scheduler table,
// and runs the normal scheduling transaction. Conductor polling calls this so message-type interpretation stays
// inside the scheduler package while main only reacts to returned ScheduleDecision values.
func (w *Worker) InsertMessageAndRunScheduling(ctx context.Context, message IncomingMessage) ([]shared.ScheduleDecision, error) {
	parsedMessage, err := parseIncomingMessageForScheduling(message.RawBody)
	if err != nil {
		return nil, err
	}

	switch parsedMessage.MessageType {
	case shared.ScheduleMessageTypeIncident:
		logSchedulerDebug("inserting incident message", "account", parsedMessage.AccountNumber, "body_bytes", len(message.RawBody))
		if err := w.InsertAlarmMessages(ctx, []IncomingMessage{{
			RawBody:       message.RawBody,
			AccountNumber: parsedMessage.AccountNumber,
		}}); err != nil {
			return nil, err
		}
	case shared.ScheduleMessageTypeTicket:
		logSchedulerDebug("inserting ticket message", "body_bytes", len(message.RawBody), "description_bytes", len(parsedMessage.Text))
		if err := w.InsertTicketMessages(ctx, []IncomingMessage{{
			RawBody:       message.RawBody,
			AccountNumber: parsedMessage.AccountNumber,
		}}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported scheduler message type %q", parsedMessage.MessageType)
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
	if len(chain) == 0 {
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
		MessageType:   shared.ScheduleMessageTypeIncident,
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

		ticketDecisionText := row.RawMessageBody
		parsedTicketMessage, err := parseTicketMessageForScheduling(row.RawMessageBody)
		if err == nil && strings.TrimSpace(parsedTicketMessage.Text) != "" {
			ticketDecisionText = parsedTicketMessage.Text
		}

		decisions = append(decisions, shared.ScheduleDecision{
			ToSchedule:    true,
			Text:          ticketDecisionText,
			AccountNumber: row.AwsAccountNumber,
			MessageType:   shared.ScheduleMessageTypeTicket,
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

// parseIncomingMessageForScheduling performs the scheduler-owned message classification for a raw SQS body.
// It recognizes CloudWatch alarm incidents and ticket payloads, returning only the fields needed to persist the row
// and build ScheduleDecision text after RunScheduling has decided to start work.
func parseIncomingMessageForScheduling(rawMessageBody string) (parsedIncomingMessage, error) {
	if incidentMessage, err := parseIncidentMessageForScheduling(rawMessageBody); err == nil {
		return incidentMessage, nil
	}
	if ticketMessage, err := parseTicketMessageForScheduling(rawMessageBody); err == nil {
		return ticketMessage, nil
	}

	return parsedIncomingMessage{}, errors.New("unsupported scheduler message shape")
}

// parseIncidentMessageForScheduling extracts the minimum CloudWatch EventBridge fields needed by alarm chaining.
// The scheduler stores the original JSON body, but account number is pulled out here because the alarm table groups
// chains by account before producing incident decisions.
func parseIncidentMessageForScheduling(rawMessageBody string) (parsedIncomingMessage, error) {
	var cloudWatchAlarmMessage struct {
		Account    string `json:"account"`
		DetailType string `json:"detail-type"`
		Source     string `json:"source"`
	}
	if err := json.Unmarshal([]byte(rawMessageBody), &cloudWatchAlarmMessage); err != nil {
		return parsedIncomingMessage{}, err
	}
	if cloudWatchAlarmMessage.Source != "aws.cloudwatch" || cloudWatchAlarmMessage.DetailType != "CloudWatch Alarm State Change" {
		return parsedIncomingMessage{}, errors.New("not a cloudwatch alarm incident")
	}
	accountNumber := strings.TrimSpace(cloudWatchAlarmMessage.Account)
	if accountNumber == "" {
		return parsedIncomingMessage{}, errors.New("cloudwatch alarm incident is missing account number")
	}

	return parsedIncomingMessage{
		MessageType:   shared.ScheduleMessageTypeIncident,
		Text:          rawMessageBody,
		AccountNumber: accountNumber,
	}, nil
}

// parseTicketMessageForScheduling recognizes the Jira-like ticket shape documented in docs/message-shapes.md.
// It extracts only description text for the eventual agent prompt while leaving the raw body available in the
// database for later inspection or richer ticket parsing.
func parseTicketMessageForScheduling(rawMessageBody string) (parsedIncomingMessage, error) {
	var ticketMessage struct {
		ID     string `json:"id"`
		Key    string `json:"key"`
		Fields struct {
			Description json.RawMessage `json:"description"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(rawMessageBody), &ticketMessage); err != nil {
		return parsedIncomingMessage{}, err
	}
	if strings.TrimSpace(ticketMessage.ID) == "" && strings.TrimSpace(ticketMessage.Key) == "" {
		return parsedIncomingMessage{}, errors.New("ticket is missing id and key")
	}
	descriptionText := ticketDescriptionText(ticketMessage.Fields.Description)
	if descriptionText == "" {
		return parsedIncomingMessage{}, errors.New("ticket is missing description text")
	}

	return parsedIncomingMessage{
		MessageType: shared.ScheduleMessageTypeTicket,
		Text:        descriptionText,
	}, nil
}

// ticketDescriptionText flattens the ticket description field into prompt text.
// Jira-style descriptions are nested document JSON, so this walks the structure and joins every "text" field while
// also accepting a plain string description for simpler test and smoke payloads.
func ticketDescriptionText(description json.RawMessage) string {
	if len(description) == 0 {
		return ""
	}

	var plainDescription string
	if err := json.Unmarshal(description, &plainDescription); err == nil {
		return strings.TrimSpace(plainDescription)
	}

	var descriptionDocument any
	if err := json.Unmarshal(description, &descriptionDocument); err != nil {
		return ""
	}

	var textParts []string
	collectTicketDescriptionTextParts(descriptionDocument, &textParts)
	return strings.TrimSpace(strings.Join(textParts, "\n"))
}

// collectTicketDescriptionTextParts recursively walks a ticket description document looking for text leaves.
// It is deliberately small and schema-light because scheduling only needs enough text to start an agent, not a full
// Jira document renderer.
func collectTicketDescriptionTextParts(descriptionValue any, textParts *[]string) {
	switch value := descriptionValue.(type) {
	case map[string]any:
		if textValue, ok := value["text"].(string); ok && strings.TrimSpace(textValue) != "" {
			*textParts = append(*textParts, strings.TrimSpace(textValue))
		}
		for _, nestedValue := range value {
			collectTicketDescriptionTextParts(nestedValue, textParts)
		}
	case []any:
		for _, nestedValue := range value {
			collectTicketDescriptionTextParts(nestedValue, textParts)
		}
	}
}
