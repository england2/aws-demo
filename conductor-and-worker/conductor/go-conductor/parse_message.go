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
	ID         string `json:"id"`
	Account    string `json:"account"`
	DetailType string `json:"detail-type"`
	Source     string `json:"source"`
	Time       string `json:"time"`
	Detail     struct {
		AlarmName string `json:"alarmName"`
		State     struct {
			Value string `json:"value"`
		} `json:"state"`
		Configuration struct {
			Metrics []struct {
				MetricStat struct {
					Period int64 `json:"period"`
				} `json:"metricStat"`
			} `json:"metrics"`
		} `json:"configuration"`
	} `json:"detail"`
}

// ParseSQSMessageBody classifies a raw SQS JSON body into the small normalized shape used by callers.
// It can run after parseSQSMessage has preserved Body, and it intentionally returns MessageTypeUnknown
// for malformed or unsupported payloads so the caller can decide whether to retry, delete, or log the delivery.
func ParseSQSMessageBody(body []byte) ParsedSQSMessage {
	parsed := ParsedSQSMessage{
		MessageType: MessageTypeUnknown,
	}

	var event eventBridgeCloudWatchAlarmBody
	if err := json.Unmarshal(body, &event); err != nil {
		return parsed
	}

	parsed.ExternalEventID = stringPtrIfNotEmpty(event.ID)
	parsed.AccountNumber = stringPtrIfNotEmpty(event.Account)

	if event.Source != "aws.cloudwatch" || event.DetailType != "CloudWatch Alarm State Change" {
		return parsed
	}

	parsed.MessageType = MessageTypeCloudWatchAlarm
	parsed.CloudWatchAlarmName = stringPtrIfNotEmpty(event.Detail.AlarmName)
	parsed.CloudWatchState = stringPtrIfNotEmpty(event.Detail.State.Value)

	if event.Time != "" {
		if eventTime, err := time.Parse(time.RFC3339, event.Time); err == nil {
			parsed.EventTime = &eventTime
		}
	}

	for _, metric := range event.Detail.Configuration.Metrics {
		if metric.MetricStat.Period > 0 {
			period := metric.MetricStat.Period
			parsed.AlarmPeriodSeconds = &period
			break
		}
	}

	return parsed
}

// stringPtrIfNotEmpty normalizes parsed JSON strings into optional values for ParsedSQSMessage.
// ParseSQSMessageBody calls it only after JSON unmarshalling succeeds, so empty EventBridge fields do not look
// like meaningful alarm metadata to downstream prompt or logging code.
func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
