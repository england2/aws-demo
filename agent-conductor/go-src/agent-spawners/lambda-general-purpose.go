package agentspawners

import (
	"encoding/json"

	"agent-orchestrator/agent-spawners/lib"
)

type LambdaGeneralPurposeAgent struct{}

func (a LambdaGeneralPurposeAgent) Spec() lib.AgentSpawnerSpec {
	return lib.AgentSpawnerSpec{
		Name:        "general-purpose-lambda",
		Description: "General purpose Lambda-backed agent spawner.",
	}
}

func (a LambdaGeneralPurposeAgent) DecideRunSQSMessage(message lib.SQSMessage) (lib.AgentMatch, error) {
	match := lib.AgentMatch{
		AgentSpawnerName: a.Spec().Name,
	}

	var event lib.CloudWatchAlarmEvent
	if err := json.Unmarshal(message.Body, &event); err != nil {
		return match, nil
	}

	if event.Source != "aws.cloudwatch" || event.DetailType != "CloudWatch Alarm State Change" {
		return match, nil
	}

	if event.Detail.AlarmName != "debian-cpu-spin-high-cpu" || event.Detail.State.Value != "ALARM" {
		return match, nil
	}

	match.AddJobPoint(1, "Matched with 'debian-cpu-spin-high-cpu' CloudWatch alert")

	return match, nil
}
