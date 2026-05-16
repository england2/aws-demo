package main

import (
	"encoding/json"
	"time"
)

const (
	MessageTypeUnknown         = "unknown"
	MessageTypeCloudWatchAlarm = "cloudwatch_alarm"
)

// ParsedSQSMessage stores normalized fields extracted from an inbound SQS body.
// Callers can use these deterministic fields when building prompts or logs.
type ParsedSQSMessage struct {
	ExternalEventID     *string
	AccountNumber       *string
	MessageType         string
	CloudWatchAlarmName *string
	CloudWatchState     *string
	EventTime           *time.Time
	AlarmPeriodSeconds  *int64
}

// eventBridgeCloudWatchAlarmBody models the EventBridge CloudWatch alarm payload.
// It intentionally includes only fields needed for classification and chain decisions.
type eventBridgeCloudWatchAlarmBody struct {
	ID         string                     `json:"id"`
	Account    string                     `json:"account"`
	DetailType string                     `json:"detail-type"`
	Source     string                     `json:"source"`
	Time       string                     `json:"time"`
	Detail     cloudWatchAlarmEventDetail `json:"detail"`
}

type cloudWatchAlarmEventDetail struct {
	AlarmName     string                       `json:"alarmName"`
	State         cloudWatchAlarmState         `json:"state"`
	Configuration cloudWatchAlarmConfiguration `json:"configuration"`
}

type cloudWatchAlarmState struct {
	Value string `json:"value"`
}

type cloudWatchAlarmConfiguration struct {
	Metrics []cloudWatchAlarmMetric `json:"metrics"`
}

type cloudWatchAlarmMetric struct {
	MetricStat cloudWatchMetricStat `json:"metricStat"`
}

type cloudWatchMetricStat struct {
	Period int64 `json:"period"`
}

// ParseSQSMessageBody classifies a raw SQS JSON body into the small normalized shape used by callers.
// It can run after parseSQSMessage has preserved Body, and it intentionally returns MessageTypeUnknown
// for malformed or unsupported payloads so the caller can decide whether to retry, delete, or log the delivery.
func ParseSQSMessageBody(body []byte) ParsedSQSMessage {
	var event eventBridgeCloudWatchAlarmBody
	if err := json.Unmarshal(body, &event); err != nil {
		return ParsedSQSMessage{MessageType: MessageTypeUnknown}
	}

	return event.Normalize()
}

func (event eventBridgeCloudWatchAlarmBody) Normalize() ParsedSQSMessage {
	parsed := ParsedSQSMessage{
		MessageType:     MessageTypeUnknown,
		ExternalEventID: ptrNonEmpty(event.ID),
		AccountNumber:   ptrNonEmpty(event.Account),
	}

	if event.Source != "aws.cloudwatch" || event.DetailType != "CloudWatch Alarm State Change" {
		return parsed
	}

	parsed.MessageType = MessageTypeCloudWatchAlarm
	parsed.CloudWatchAlarmName = ptrNonEmpty(event.Detail.AlarmName)
	parsed.CloudWatchState = ptrNonEmpty(event.Detail.State.Value)
	parsed.EventTime = parseRFC3339Ptr(event.Time)
	parsed.AlarmPeriodSeconds = firstMetricPeriod(event.Detail.Configuration.Metrics)

	return parsed
}

// ptrNonEmpty normalizes parsed JSON strings into optional values for ParsedSQSMessage.
// Normalize calls it only after JSON unmarshalling succeeds, so empty EventBridge fields do not look
// like meaningful alarm metadata to downstream prompt or logging code.
func ptrNonEmpty(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func parseRFC3339Ptr(value string) *time.Time {
	if value == "" {
		return nil
	}

	parsedTime, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}

	return &parsedTime
}

func firstMetricPeriod(metrics []cloudWatchAlarmMetric) *int64 {
	for _, metric := range metrics {
		if metric.MetricStat.Period > 0 {
			period := metric.MetricStat.Period
			return &period
		}
	}

	return nil
}
