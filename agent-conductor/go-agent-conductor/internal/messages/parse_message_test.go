package messages

import "testing"

// TestParseSQSMessageBodyCloudWatchAlarm verifies EventBridge alarm fields are normalized.
// The database intake logic depends on these parsed fields for chain and spawn decisions.
func TestParseSQSMessageBodyCloudWatchAlarm(t *testing.T) {
	body := []byte(`{
		"id": "event-1",
		"detail-type": "CloudWatch Alarm State Change",
		"source": "aws.cloudwatch",
		"time": "2026-05-01T04:15:44Z",
		"detail": {
			"alarmName": "debian-cpu-spin-high-cpu",
			"state": {"value": "ALARM"},
			"configuration": {
				"metrics": [
					{"metricStat": {"period": 20}}
				]
			}
		}
	}`)

	parsed := ParseSQSMessageBody(body)

	if parsed.MessageType != MessageTypeCloudWatchAlarm {
		t.Fatalf("MessageType = %q, want %q", parsed.MessageType, MessageTypeCloudWatchAlarm)
	}
	if parsed.ExternalEventID == nil || *parsed.ExternalEventID != "event-1" {
		t.Fatalf("ExternalEventID = %v, want event-1", parsed.ExternalEventID)
	}
	if parsed.CloudWatchAlarmName == nil || *parsed.CloudWatchAlarmName != "debian-cpu-spin-high-cpu" {
		t.Fatalf("CloudWatchAlarmName = %v", parsed.CloudWatchAlarmName)
	}
	if parsed.CloudWatchState == nil || *parsed.CloudWatchState != "ALARM" {
		t.Fatalf("CloudWatchState = %v", parsed.CloudWatchState)
	}
	if parsed.EventTime == nil {
		t.Fatalf("EventTime is nil")
	}
	if parsed.AlarmPeriodSeconds == nil || *parsed.AlarmPeriodSeconds != 20 {
		t.Fatalf("AlarmPeriodSeconds = %v, want 20", parsed.AlarmPeriodSeconds)
	}
}

// TestParseSQSMessageBodyUnknownForInvalidJSON ensures malformed JSON is not fatal here.
// The parser classifies it as unknown and leaves quarantine/ignore policy to callers.
func TestParseSQSMessageBodyUnknownForInvalidJSON(t *testing.T) {
	parsed := ParseSQSMessageBody([]byte(`not-json`))

	if parsed.MessageType != MessageTypeUnknown {
		t.Fatalf("MessageType = %q, want %q", parsed.MessageType, MessageTypeUnknown)
	}
}
