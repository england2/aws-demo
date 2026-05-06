package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// TestBuildRunTaskInputUsesTerraformTaskDefinitionAndJobOverrides locks core ECS request shape.
// It verifies Terraform-created task metadata and per-job environment overrides are wired.
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
	if input.ClientToken != nil {
		t.Fatalf("ClientToken = %q, want nil so repeated local agent job IDs do not conflict", aws.ToString(input.ClientToken))
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
	got := environmentMap(environment)
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("environment %s = %q, want %q", key, got[key], value)
		}
	}
	if _, ok := got["DEBUG_SSH_ENABLED"]; ok {
		t.Fatalf("DEBUG_SSH_ENABLED should not be present by default")
	}
}

// TestBuildRunTaskInputIncludesDebugSSHEnvWhenEnabled verifies optional debug SSH env passthrough.
// This protects the manual ECS Exec/SSH debugging path used during Fargate development.
func TestBuildRunTaskInputIncludesDebugSSHEnvWhenEnabled(t *testing.T) {
	input, err := BuildRunTaskInput(AgentFargateJobConfig{
		AWSFargateSpawnConfig: AWSFargateSpawnConfig{
			Region:         "us-west-2",
			Cluster:        "ecs-cluster-agent-fargate",
			TaskDefinition: "agent-fargate",
			ContainerName:  "agent-fargate",
			Subnets:        []string{"subnet-1"},
			SecurityGroups: []string{"sg-1"},
		},
		RuntimeEnv: AgentFargateRuntimeEnv{
			AgentJobID:              "42",
			AgentName:               "agent-fargate-codex",
			EventsQueueURL:          "https://sqs.us-west-2.amazonaws.com/204772699175/agent-fargate-events",
			DebugSSHEnabled:         true,
			DebugSSHPublicKeySecret: "debug_public_ssh_key",
		},
	})
	if err != nil {
		t.Fatalf("BuildRunTaskInput error: %v", err)
	}

	got := environmentMap(input.Overrides.ContainerOverrides[0].Environment)
	if got["DEBUG_SSH_ENABLED"] != "true" {
		t.Fatalf("DEBUG_SSH_ENABLED = %q, want true", got["DEBUG_SSH_ENABLED"])
	}
	if got["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"] != "debug_public_ssh_key" {
		t.Fatalf("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME = %q", got["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"])
	}
}

// environmentMap converts ECS key-value pairs to a map for readable test assertions.
// It mirrors how the wrapper sees environment variables inside the Fargate container.
func environmentMap(environment []ecstypes.KeyValuePair) map[string]string {
	got := make(map[string]string, len(environment))
	for _, pair := range environment {
		got[aws.ToString(pair.Name)] = aws.ToString(pair.Value)
	}
	return got
}
