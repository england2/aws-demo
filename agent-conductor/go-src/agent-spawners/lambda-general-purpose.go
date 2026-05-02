package agentspawners

import (
	"encoding/json"

	"agent-orchestrator/agent-spawners/lib"
)

type LambdaGeneralPurposeAgent struct{}

func (a LambdaGeneralPurposeAgent) Name() string {
	return "general-purpose-lambda"
}

// TODO configure to NOT run on any tickets that failed previously

// implementing the DecideRunSQSMessage
//
// ai-- this function should:
// - start with an empty JobPoints struct
// - if we match with the cpu spin alert, we will add 1 job points, with the reason "Matched with 'debian-cpu-spin-high-cpu' CloudWatch alert"
//
//
func (a LambdaGeneralPurposeAgent) DecideRunSQSMessage(message lib.SQSMessage) (lib.AgentMatch, error) {
	var event lib.CloudWatchAlarmEvent
	if err := json.Unmarshal(message.Body, &event); err != nil {
		return lib.AgentMatch{}, nil
	}

	if event.Source != "aws.cloudwatch" || event.DetailType != "CloudWatch Alarm State Change" {
		return lib.AgentMatch{}, nil
	}

	if event.Detail.AlarmName != "debian-cpu-spin-high-cpu" || event.Detail.State.Value != "ALARM" {
		return lib.AgentMatch{}, nil
	}

	return lib.AgentMatch{
		AgentName:   a.Name(),
		JobPriority: 100,
	}, nil
}
