package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestBuildRunTaskInputUsesTerraformTaskDefinitionAndJobOverrides(t *testing.T) {
	input, err := BuildRunTaskInput(AgentFargateJobConfig{
		AWSFargateSpawnConfig: AWSFargateSpawnConfig{
			Region:         "us-west-2",
			Cluster:        "ecs-cluster-agent-fargate",
			TaskDefinition: "agent-fargate",
			ContainerName:  "agent-fargate",
			Subnets:        []string{"subnet-1"},
			SecurityGroups: []string{"sg-1"},
			AssignPublicIP: true,
		},
		RuntimeEnv: AgentFargateRuntimeEnv{
			AgentJobID:     "42",
			AgentName:      "agent-fargate-codex",
			Prompt:         "do the work",
			EventsQueueURL: "https://sqs.us-west-2.amazonaws.com/204772699175/agent-fargate-events",
		},
	})
	if err != nil {
		t.Fatalf("BuildRunTaskInput error: %v", err)
	}

	if aws.ToString(input.Cluster) != "ecs-cluster-agent-fargate" {
		t.Fatalf("Cluster = %q", aws.ToString(input.Cluster))
	}
	if aws.ToString(input.TaskDefinition) != "agent-fargate" {
		t.Fatalf("TaskDefinition = %q", aws.ToString(input.TaskDefinition))
	}
	if input.LaunchType != ecstypes.LaunchTypeFargate {
		t.Fatalf("LaunchType = %q, want FARGATE", input.LaunchType)
	}
	if aws.ToString(input.ClientToken) != "agent-job-exec-42" {
		t.Fatalf("ClientToken = %q", aws.ToString(input.ClientToken))
	}
	if !input.EnableExecuteCommand {
		t.Fatalf("EnableExecuteCommand = false, want true")
	}
	if input.NetworkConfiguration.AwsvpcConfiguration.AssignPublicIp != ecstypes.AssignPublicIpEnabled {
		t.Fatalf("AssignPublicIp = %q, want ENABLED", input.NetworkConfiguration.AwsvpcConfiguration.AssignPublicIp)
	}

	environment := input.Overrides.ContainerOverrides[0].Environment
	want := map[string]string{
		"AGENT_JOB_ID":                   "42",
		"AGENT_NAME":                     "agent-fargate-codex",
		"AGENT_PROMPT":                   "do the work",
		"AGENT_FARGATE_EVENTS_QUEUE_URL": "https://sqs.us-west-2.amazonaws.com/204772699175/agent-fargate-events",
	}
	got := make(map[string]string, len(environment))
	for _, pair := range environment {
		got[aws.ToString(pair.Name)] = aws.ToString(pair.Value)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("environment %s = %q, want %q", key, got[key], value)
		}
	}
}
