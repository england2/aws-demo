package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// TestSpawnFargateAgentBuildsRunTaskInput locks core ECS request shape.
// It verifies Terraform-created task metadata and per-job environment overrides are wired.
func TestSpawnFargateAgentBuildsRunTaskInput(t *testing.T) {
	t.Setenv("DEBUG_SSH_ENABLED", "")
	t.Setenv("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", "")

	fakeECS := &fakeFargateECSClient{
		runTaskOutput: &ecs.RunTaskOutput{
			Tasks: []ecstypes.Task{{TaskArn: aws.String("arn:aws:ecs:us-west-2:123:task/42")}},
		},
	}
	installFargateSpawnTestDoubles(t, fakeECS, AWSFargateSpawnConfig{
		Region:         "us-west-2",
		Cluster:        "ecs-cluster-agent-fargate",
		TaskDefinition: "agent-fargate",
		ContainerName:  "agent-fargate",
		Subnets:        []string{"subnet-1"},
		SecurityGroups: []string{"sg-1"},
		AssignPublicIP: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentJob := DatabaseAgentJobInfo{ID: 42, AgentName: "agent-fargate-codex"}
	message := DatabaseSQSMessageInfo{ID: 7, MessageType: "cloudwatch", RawBody: "alarm"}
	worker, err := spawnFargateAgent(ctx, agentJob, message)
	if err != nil {
		t.Fatalf("spawnFargateAgent error: %v", err)
	}
	if worker.TaskARN != "arn:aws:ecs:us-west-2:123:task/42" {
		t.Fatalf("TaskARN = %q", worker.TaskARN)
	}

	input := fakeECS.runTaskInput
	if input == nil {
		t.Fatalf("RunTask was not called")
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
		t.Fatalf("ClientToken = %q, want nil so repeated local agent job IDs do not conflict", aws.ToString(input.ClientToken))
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
	wantEnvironment := map[string]string{
		"AGENT_JOB_ID": "42",
		"AGENT_NAME":   "agent-fargate-codex",
		"AGENT_PROMPT": buildAgentPrompt(agentJob, message),
	}
	for key, value := range wantEnvironment {
		if gotEnvironment[key] != value {
			t.Fatalf("environment %s = %q, want %q", key, gotEnvironment[key], value)
		}
	}
	if _, ok := gotEnvironment["DEBUG_SSH_ENABLED"]; ok {
		t.Fatalf("DEBUG_SSH_ENABLED should not be present by default")
	}
}

// TestSpawnFargateAgentIncludesDebugSSHEnvWhenEnabled verifies optional debug SSH env passthrough.
// This protects the manual ECS Exec/SSH debugging path used during Fargate development.
func TestSpawnFargateAgentIncludesDebugSSHEnvWhenEnabled(t *testing.T) {
	t.Setenv("DEBUG_SSH_ENABLED", "true")
	t.Setenv("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", "debug_public_ssh_key")

	fakeECS := &fakeFargateECSClient{
		runTaskOutput: &ecs.RunTaskOutput{
			Tasks: []ecstypes.Task{{TaskArn: aws.String("arn:aws:ecs:us-west-2:123:task/42")}},
		},
	}
	installFargateSpawnTestDoubles(t, fakeECS, validTestFargateSpawnConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := spawnFargateAgent(ctx, DatabaseAgentJobInfo{ID: 42, AgentName: "agent-fargate-codex"}, DatabaseSQSMessageInfo{ID: 7}); err != nil {
		t.Fatalf("spawnFargateAgent error: %v", err)
	}

	got := environmentMap(fakeECS.runTaskInput.Overrides.ContainerOverrides[0].Environment)
	if got["DEBUG_SSH_ENABLED"] != "true" {
		t.Fatalf("DEBUG_SSH_ENABLED = %q, want true", got["DEBUG_SSH_ENABLED"])
	}
	if got["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"] != "debug_public_ssh_key" {
		t.Fatalf("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME = %q", got["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"])
	}
}

// TestSpawnFargateAgentReturnsRunTaskError verifies direct spawn callers do not need a DB channel.
func TestSpawnFargateAgentReturnsRunTaskError(t *testing.T) {
	t.Setenv("DEBUG_SSH_ENABLED", "")

	fakeECS := &fakeFargateECSClient{runTaskErr: errors.New("boom")}
	installFargateSpawnTestDoubles(t, fakeECS, validTestFargateSpawnConfig())

	_, err := spawnFargateAgent(context.Background(), DatabaseAgentJobInfo{ID: 42, AgentName: "agent-fargate-codex"}, DatabaseSQSMessageInfo{ID: 7})
	if err == nil {
		t.Fatalf("spawnFargateAgent error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run Fargate task: boom") {
		t.Fatalf("error = %q", err.Error())
	}
}

// TestSpawnAndTrackAgentJobMarksSpawned verifies DB tracking remains in the conductor wrapper.
func TestSpawnAndTrackAgentJobMarksSpawned(t *testing.T) {
	t.Setenv("DEBUG_SSH_ENABLED", "")

	fakeECS := &fakeFargateECSClient{
		runTaskOutput: &ecs.RunTaskOutput{
			Tasks: []ecstypes.Task{{TaskArn: aws.String("arn:aws:ecs:us-west-2:123:task/42")}},
		},
	}
	installFargateSpawnTestDoubles(t, fakeECS, validTestFargateSpawnConfig())
	commands, seenCommands := startTestDatabaseCommandWorker(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !spawnAndTrackAgentJob(ctx, commands, DatabaseAgentJobInfo{ID: 42, AgentName: "agent-fargate-codex"}, DatabaseSQSMessageInfo{ID: 7}) {
		t.Fatalf("spawnAndTrackAgentJob returned false")
	}
	cancel()

	command := <-seenCommands
	if command.Kind != DatabaseCommandMarkAgentJobSpawned {
		t.Fatalf("database command = %q, want %q", command.Kind, DatabaseCommandMarkAgentJobSpawned)
	}
	if command.AgentJobID != 42 {
		t.Fatalf("database AgentJobID = %d, want 42", command.AgentJobID)
	}
	if command.ECSTaskARN != "arn:aws:ecs:us-west-2:123:task/42" {
		t.Fatalf("database ECSTaskARN = %q", command.ECSTaskARN)
	}
}

func installFargateSpawnTestDoubles(t *testing.T, fakeECS *fakeFargateECSClient, spawnConfig AWSFargateSpawnConfig) {
	t.Helper()

	originalSpawnConfig := adhocAWSFargateSpawnConfig
	originalLoadFargateAWSConfig := loadFargateAWSConfig
	originalNewFargateECSClient := newFargateECSClient

	adhocAWSFargateSpawnConfig = spawnConfig
	loadFargateAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
		if region != spawnConfig.Region {
			t.Fatalf("loadFargateAWSConfig region = %q, want %q", region, spawnConfig.Region)
		}
		return aws.Config{Region: region}, nil
	}
	newFargateECSClient = func(awsConfig aws.Config) ecsFargateClient {
		if awsConfig.Region != spawnConfig.Region {
			t.Fatalf("newFargateECSClient region = %q, want %q", awsConfig.Region, spawnConfig.Region)
		}
		return fakeECS
	}

	t.Cleanup(func() {
		adhocAWSFargateSpawnConfig = originalSpawnConfig
		loadFargateAWSConfig = originalLoadFargateAWSConfig
		newFargateECSClient = originalNewFargateECSClient
	})
}

func startTestDatabaseCommandWorker(t *testing.T) (chan<- DatabaseCommand, <-chan DatabaseCommand) {
	t.Helper()

	commands := make(chan DatabaseCommand)
	seenCommands := make(chan DatabaseCommand, 10)
	go func() {
		for command := range commands {
			seenCommands <- command
			command.Reply <- DatabaseCommandResult{}
		}
	}()

	t.Cleanup(func() {
		close(commands)
	})

	return commands, seenCommands
}

func validTestFargateSpawnConfig() AWSFargateSpawnConfig {
	return AWSFargateSpawnConfig{
		Region:         "us-west-2",
		Cluster:        "ecs-cluster-agent-fargate",
		TaskDefinition: "agent-fargate",
		ContainerName:  "agent-fargate",
		Subnets:        []string{"subnet-1"},
		SecurityGroups: []string{"sg-1"},
		AssignPublicIP: true,
	}
}

type fakeFargateECSClient struct {
	runTaskInput  *ecs.RunTaskInput
	runTaskOutput *ecs.RunTaskOutput
	runTaskErr    error
}

func (client *fakeFargateECSClient) RunTask(ctx context.Context, input *ecs.RunTaskInput, optFns ...func(*ecs.Options)) (*ecs.RunTaskOutput, error) {
	client.runTaskInput = input
	if client.runTaskErr != nil {
		return nil, client.runTaskErr
	}
	if client.runTaskOutput != nil {
		return client.runTaskOutput, nil
	}
	return &ecs.RunTaskOutput{}, nil
}

func (client *fakeFargateECSClient) DescribeTasks(ctx context.Context, input *ecs.DescribeTasksInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	return &ecs.DescribeTasksOutput{
		Tasks: []ecstypes.Task{{LastStatus: aws.String("RUNNING")}},
	}, nil
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
