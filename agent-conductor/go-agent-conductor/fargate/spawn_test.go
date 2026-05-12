package fargate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestBuildRunTaskInput(t *testing.T) {
	request := validSpawnRequest()

	input, err := BuildRunTaskInput(request)
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
	if aws.ToInt32(input.Count) != 1 {
		t.Fatalf("Count = %d, want 1", aws.ToInt32(input.Count))
	}
	if input.ClientToken != nil {
		t.Fatalf("ClientToken = %q, want nil", aws.ToString(input.ClientToken))
	}
	if !input.EnableExecuteCommand {
		t.Fatalf("EnableExecuteCommand = false, want true")
	}
	if !input.EnableECSManagedTags {
		t.Fatalf("EnableECSManagedTags = false, want true")
	}

	awsvpcConfiguration := input.NetworkConfiguration.AwsvpcConfiguration
	if awsvpcConfiguration.AssignPublicIp != ecstypes.AssignPublicIpEnabled {
		t.Fatalf("AssignPublicIp = %q, want ENABLED", awsvpcConfiguration.AssignPublicIp)
	}
	if got := strings.Join(awsvpcConfiguration.Subnets, ","); got != "subnet-1" {
		t.Fatalf("Subnets = %q, want subnet-1", got)
	}
	if got := strings.Join(awsvpcConfiguration.SecurityGroups, ","); got != "sg-1" {
		t.Fatalf("SecurityGroups = %q, want sg-1", got)
	}

	containerOverride := input.Overrides.ContainerOverrides[0]
	if aws.ToString(containerOverride.Name) != "agent-fargate" {
		t.Fatalf("ContainerOverride.Name = %q", aws.ToString(containerOverride.Name))
	}
	gotEnvironment := environmentMap(containerOverride.Environment)
	if gotEnvironment["AGENT_NAME"] != "agent-fargate-codex" {
		t.Fatalf("AGENT_NAME = %q", gotEnvironment["AGENT_NAME"])
	}
	if gotEnvironment["AGENT_PROMPT"] != "do work" {
		t.Fatalf("AGENT_PROMPT = %q", gotEnvironment["AGENT_PROMPT"])
	}
}

func TestWithDebugSSHEnvironment(t *testing.T) {
	t.Setenv("DEBUG_SSH_ENABLED", "true")
	t.Setenv("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", "debug_public_ssh_key")

	environment := WithDebugSSHEnvironment(map[string]string{
		"AGENT_NAME":   "agent-fargate-codex",
		"AGENT_PROMPT": "do work",
	})

	if environment["DEBUG_SSH_ENABLED"] != "true" {
		t.Fatalf("DEBUG_SSH_ENABLED = %q", environment["DEBUG_SSH_ENABLED"])
	}
	if environment["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"] != "debug_public_ssh_key" {
		t.Fatalf("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME = %q", environment["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"])
	}
}

func TestSpawnReturnsTaskARN(t *testing.T) {
	fakeECS := &fakeECSClient{
		runTaskOutput: &ecs.RunTaskOutput{
			Tasks: []ecstypes.Task{{TaskArn: aws.String("arn:aws:ecs:us-west-2:123:task/42")}},
		},
	}
	installSpawnTestDoubles(t, fakeECS, "us-west-2")

	result, err := Spawn(context.Background(), validSpawnRequest())
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}
	if result.TaskARN != "arn:aws:ecs:us-west-2:123:task/42" {
		t.Fatalf("TaskARN = %q", result.TaskARN)
	}
	if fakeECS.runTaskInput == nil {
		t.Fatalf("RunTask was not called")
	}
}

func TestSpawnReturnsRunTaskError(t *testing.T) {
	installSpawnTestDoubles(t, &fakeECSClient{runTaskErr: errors.New("boom")}, "us-west-2")

	_, err := Spawn(context.Background(), validSpawnRequest())
	if err == nil {
		t.Fatalf("Spawn error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run Fargate task: boom") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestSpawnReturnsECSFailures(t *testing.T) {
	installSpawnTestDoubles(t, &fakeECSClient{
		runTaskOutput: &ecs.RunTaskOutput{
			Failures: []ecstypes.Failure{{Reason: aws.String("RESOURCE:MEMORY")}},
		},
	}, "us-west-2")

	_, err := Spawn(context.Background(), validSpawnRequest())
	if err == nil {
		t.Fatalf("Spawn error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run Fargate task failed") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestSpawnRequiresOneTaskARN(t *testing.T) {
	installSpawnTestDoubles(t, &fakeECSClient{runTaskOutput: &ecs.RunTaskOutput{}}, "us-west-2")

	_, err := Spawn(context.Background(), validSpawnRequest())
	if err == nil {
		t.Fatalf("Spawn error = nil, want error")
	}
	if !strings.Contains(err.Error(), "expected one Fargate task") {
		t.Fatalf("error = %q", err.Error())
	}
}

func installSpawnTestDoubles(t *testing.T, fakeECS *fakeECSClient, wantRegion string) {
	t.Helper()

	originalLoadAWSConfig := loadAWSConfig
	originalNewECSClient := newECSClient

	loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
		if region != wantRegion {
			t.Fatalf("loadAWSConfig region = %q, want %q", region, wantRegion)
		}
		return aws.Config{Region: region}, nil
	}
	newECSClient = func(awsConfig aws.Config) ecsClient {
		if awsConfig.Region != wantRegion {
			t.Fatalf("newECSClient region = %q, want %q", awsConfig.Region, wantRegion)
		}
		return fakeECS
	}

	t.Cleanup(func() {
		loadAWSConfig = originalLoadAWSConfig
		newECSClient = originalNewECSClient
	})
}

func validSpawnRequest() SpawnRequest {
	return SpawnRequest{
		Config: SpawnConfig{
			Region:         "us-west-2",
			Cluster:        "ecs-cluster-agent-fargate",
			TaskDefinition: "agent-fargate",
			ContainerName:  "agent-fargate",
			Subnets:        []string{"subnet-1"},
			SecurityGroups: []string{"sg-1"},
			AssignPublicIP: true,
		},
		Environment: map[string]string{
			"AGENT_NAME":   "agent-fargate-codex",
			"AGENT_PROMPT": "do work",
		},
	}
}

type fakeECSClient struct {
	runTaskInput  *ecs.RunTaskInput
	runTaskOutput *ecs.RunTaskOutput
	runTaskErr    error
}

func (client *fakeECSClient) RunTask(ctx context.Context, input *ecs.RunTaskInput, optFns ...func(*ecs.Options)) (*ecs.RunTaskOutput, error) {
	client.runTaskInput = input
	if client.runTaskErr != nil {
		return nil, client.runTaskErr
	}
	if client.runTaskOutput != nil {
		return client.runTaskOutput, nil
	}
	return &ecs.RunTaskOutput{}, nil
}

func environmentMap(environment []ecstypes.KeyValuePair) map[string]string {
	got := make(map[string]string, len(environment))
	for _, pair := range environment {
		got[aws.ToString(pair.Name)] = aws.ToString(pair.Value)
	}
	return got
}
