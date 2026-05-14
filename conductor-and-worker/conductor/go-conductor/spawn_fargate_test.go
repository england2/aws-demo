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

// TestBuildRunTaskInput verifies that a complete FargateWorkerSpawnRequest turns into the ECS request shape expected by RunTask.
// It sits downstream of validSpawnRequest and upstream of Spawn, checking the config and environment mapping without
// loading AWS credentials or calling ECS.
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
	if gotEnvironment["CONDUCTOR_GRPC_SERVER_ADDR"] != "localhost:50055" {
		t.Fatalf("CONDUCTOR_GRPC_SERVER_ADDR = %q", gotEnvironment["CONDUCTOR_GRPC_SERVER_ADDR"])
	}
	if gotEnvironment["WORKER_ID"] != "worker-test" {
		t.Fatalf("WORKER_ID = %q", gotEnvironment["WORKER_ID"])
	}
}

// counter-factural confirmed
// TestBuildFargateSpawnRequestUsesWorkerIdentityEnvironment verifies the conductor-to-Fargate handoff.
// It runs after prepareWorkerSpawnConfig would have established the worker ID and confirms Fargate receives only
// the identity env needed to handshake and request its server-side work files.
func TestBuildFargateSpawnRequestUsesWorkerIdentityEnvironment(t *testing.T) {
	request := BuildFargateSpawnRequest(workerSpawnConfig{
		ConductorGrpcServerAddr: "localhost:50055",
		WorkerID:                "worker-test",
	}, validSpawnConfig())

	if request.Infrastructure.Cluster != "ecs-cluster-agent-fargate" {
		t.Fatalf("Cluster = %q", request.Infrastructure.Cluster)
	}
	if request.Environment["CONDUCTOR_GRPC_SERVER_ADDR"] != "localhost:50055" {
		t.Fatalf("CONDUCTOR_GRPC_SERVER_ADDR = %q", request.Environment["CONDUCTOR_GRPC_SERVER_ADDR"])
	}
	if request.Environment["WORKER_ID"] != "worker-test" {
		t.Fatalf("WORKER_ID = %q", request.Environment["WORKER_ID"])
	}
	if _, ok := request.Environment["AGENT_PROMPT"]; ok {
		t.Fatalf("AGENT_PROMPT should not be passed to Fargate worker env")
	}
}

// TestWithDebugSSHEnvironment verifies optional debug SSH env is appended only after the base worker env exists.
// It exercises the same helper used by HandleSQSMessage and the smoke entrypoint before BuildRunTaskInput serializes
// environment values into container overrides.
func TestWithDebugSSHEnvironment(t *testing.T) {
	t.Setenv("DEBUG_SSH_ENABLED", "true")
	t.Setenv("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", "debug_public_ssh_key")

	environment := WithDebugSSHEnvironment(map[string]string{
		"CONDUCTOR_GRPC_SERVER_ADDR": "localhost:50055",
		"WORKER_ID":                  "worker-test",
	})

	if environment["DEBUG_SSH_ENABLED"] != "true" {
		t.Fatalf("DEBUG_SSH_ENABLED = %q", environment["DEBUG_SSH_ENABLED"])
	}
	if environment["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"] != "debug_public_ssh_key" {
		t.Fatalf("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME = %q", environment["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"])
	}
}

// TestSpawnReturnsTaskARN covers the successful end-to-end package flow from FargateWorkerSpawnRequest to returned ECS task ARN.
// It installs fake AWS/ECS constructors before calling Spawn, so the test observes the RunTask input that would later
// let the conductor delete the handled SQS receipt.
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

// TestSpawnReturnsRunTaskError ensures SDK call failures stop the spawn flow before any task ARN is reported.
// It uses the same valid request path as successful callers, with the ECS fake failing at the point Spawn would
// normally hand control to AWS.
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

// TestSpawnReturnsECSFailures ensures ECS-level RunTask failures are surfaced as readable errors.
// The fake client returns a RunTaskOutput with failures after input construction, matching the branch main and
// the smoke entrypoint rely on to avoid treating a failed spawn as handled.
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

// TestSpawnRequiresOneTaskARN verifies Spawn only succeeds when ECS reports exactly one task with an ARN.
// It protects the conductor's later delete-on-spawn behavior by making ambiguous or empty RunTask results fail
// before a caller can acknowledge the SQS message.
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

// installSpawnTestDoubles replaces AWS config loading and ECS client construction for tests in this package.
// It must run before Spawn is invoked, and its cleanup restores global constructor hooks so later tests see the
// normal production dataflow.
func installSpawnTestDoubles(t *testing.T, fakeECS *fakeECSClient, wantRegion string) {
	t.Helper()

	originalLoadAWSConfig := loadAWSConfig
	originalNewECSClient := newECSClient

	// This loader fake verifies Spawn asks for the expected region before client construction.
	// It replaces the SDK default credential/config chain so tests reach the ECS fake without external dependencies.
	loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
		if region != wantRegion {
			t.Fatalf("loadAWSConfig region = %q, want %q", region, wantRegion)
		}
		return aws.Config{Region: region}, nil
	}
	// This client-constructor fake connects the already-checked AWS region to the in-memory ECS client.
	// Spawn calls it after loadAWSConfig succeeds, making fakeECS the receiver for the subsequent RunTask call.
	newECSClient = func(awsConfig aws.Config) ecsClient {
		if awsConfig.Region != wantRegion {
			t.Fatalf("newECSClient region = %q, want %q", awsConfig.Region, wantRegion)
		}
		return fakeECS
	}

	// This cleanup closure restores production AWS/ECS hooks after a test finishes.
	// It runs after Spawn has used the fakes, preventing one test's constructor overrides from affecting another.
	t.Cleanup(func() {
		loadAWSConfig = originalLoadAWSConfig
		newECSClient = originalNewECSClient
	})
}

// validSpawnRequest builds the smallest complete request accepted by validateSpawnRequest.
// Tests feed it into BuildRunTaskInput and Spawn so assertions focus on one behavior at a time instead of repeating
// static ECS config and base worker environment setup.
func validSpawnRequest() FargateWorkerSpawnRequest {
	return FargateWorkerSpawnRequest{
		Infrastructure: validSpawnConfig(),
		Environment: map[string]string{
			"CONDUCTOR_GRPC_SERVER_ADDR": "localhost:50055",
			"WORKER_ID":                  "worker-test",
		},
	}
}

func validSpawnConfig() FargateInfrastructureConfig {
	return FargateInfrastructureConfig{
		Region:         "us-west-2",
		Cluster:        "ecs-cluster-agent-fargate",
		TaskDefinition: "agent-fargate",
		ContainerName:  "agent-fargate",
		Subnets:        []string{"subnet-1"},
		SecurityGroups: []string{"sg-1"},
		AssignPublicIP: true,
	}
}

type fakeECSClient struct {
	runTaskInput  *ecs.RunTaskInput
	runTaskOutput *ecs.RunTaskOutput
	runTaskErr    error
}

// RunTask records the ECS input handed to the fake client and returns the configured test output or error.
// Spawn calls this after BuildRunTaskInput and AWS config setup, letting tests assert both request construction
// and success/failure handling without network access.
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

// environmentMap converts ECS container environment pairs back into a lookup table for request assertions.
// Tests call it after BuildRunTaskInput has serialized FargateWorkerSpawnRequest.Environment, making env dataflow readable
// without depending on slice ordering.
func environmentMap(environment []ecstypes.KeyValuePair) map[string]string {
	got := make(map[string]string, len(environment))
	for _, pair := range environment {
		got[aws.ToString(pair.Name)] = aws.ToString(pair.Value)
	}
	return got
}
