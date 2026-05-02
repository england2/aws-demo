package lib

type CloudWatchAlarmEvent struct {
	Source     string `json:"source"`
	DetailType string `json:"detail-type"`
	Detail     struct {
		AlarmName string `json:"alarmName"`
		State     struct {
			Value  string `json:"value"`
			Reason string `json:"reason"`
		} `json:"state"`
	} `json:"detail"`
}
