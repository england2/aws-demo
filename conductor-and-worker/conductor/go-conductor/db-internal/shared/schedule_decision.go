package shared

type ScheduleMessageType string

const (
	ScheduleMessageTypeIncident ScheduleMessageType = "incident"
	ScheduleMessageTypeTicket   ScheduleMessageType = "ticket"
)

type ScheduleDecision struct {
	ToSchedule    bool
	Text          string
	AccountNumber string
	MessageType   ScheduleMessageType
}
