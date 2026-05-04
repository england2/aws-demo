package main

import (
	"encoding/json"
	"time"
)

const (
	MessageTypeUnknown         = "unknown"
	MessageTypeCloudWatchAlarm = "cloudwatch_alarm"
)

type ParsedSQSMessage struct {
	ExternalEventID     *string
	MessageType         string
	CloudWatchAlarmName *string
	CloudWatchState     *string
	EventTime           *time.Time
	AlarmPeriodSeconds  *int64
}

type eventBridgeCloudWatchAlarmBody struct {
	ID         string `json:"id"`
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

func ParseSQSMessageBody(body []byte) ParsedSQSMessage {
	parsed := ParsedSQSMessage{
		MessageType: MessageTypeUnknown,
	}

	var event eventBridgeCloudWatchAlarmBody
	if err := json.Unmarshal(body, &event); err != nil {
		return parsed
	}

	parsed.ExternalEventID = stringPtrIfNotEmpty(event.ID)

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

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
