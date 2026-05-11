package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

var loadFargateAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}

var newFargateECSClient = func(awsConfig aws.Config) ecsFargateClient {
	return ecs.NewFromConfig(awsConfig)
}

// spawnFargateAgent starts one ECS Fargate task and returns the in-memory worker.
// It has no database dependency so direct smoke tests can exercise the ECS spawn path.
func spawnFargateAgent(ctx context.Context, agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) (*FargateAgentWorker, error) {
	agentJobID := strconv.FormatInt(agentJob.ID, 10)
	debugSSHEnabled, debugSSHPublicKeySecret := DebugSSHRuntimeEnv()
	spawnConfig := adhocAWSFargateSpawnConfig
	runtimeEnv := AgentFargateRuntimeEnv{
		AgentJobID:              agentJobID,
		AgentName:               agentJob.AgentName,
		Prompt:                  buildAgentPrompt(agentJob, message),
		DebugSSHEnabled:         debugSSHEnabled,
		DebugSSHPublicKeySecret: debugSSHPublicKeySecret,
	}
	agentConfig := AgentFargateJobConfig{
		AWSFargateSpawnConfig: spawnConfig,
		RuntimeEnv:            runtimeEnv,
	}
	if err := validateAgentFargateJobConfig(agentConfig); err != nil {
		return nil, fmt.Errorf("configure Fargate agent: %w", err)
	}

	awsConfig, err := loadFargateAWSConfig(ctx, spawnConfig.Region)
	if err != nil {
		return nil, fmt.Errorf("load AWS config for Fargate agent: %w", err)
	}
	ecsClient := newFargateECSClient(awsConfig)

	assignPublicIP := ecstypes.AssignPublicIpDisabled
	if spawnConfig.AssignPublicIP {
		assignPublicIP = ecstypes.AssignPublicIpEnabled
	}

	environment := []ecstypes.KeyValuePair{
		{Name: aws.String("AGENT_JOB_ID"), Value: aws.String(runtimeEnv.AgentJobID)},
		{Name: aws.String("AGENT_NAME"), Value: aws.String(runtimeEnv.AgentName)},
		{Name: aws.String("AGENT_PROMPT"), Value: aws.String(runtimeEnv.Prompt)},
	}
	if runtimeEnv.DebugSSHEnabled {
		secretName := strings.TrimSpace(runtimeEnv.DebugSSHPublicKeySecret)
		if secretName == "" {
			secretName = "debug_public_ssh_key"
		}
		environment = append(
			environment,
			ecstypes.KeyValuePair{Name: aws.String("DEBUG_SSH_ENABLED"), Value: aws.String("true")},
			ecstypes.KeyValuePair{Name: aws.String("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"), Value: aws.String(secretName)},
		)
	}

	awsvpcConfiguration := ecstypes.AwsVpcConfiguration{
		Subnets:        spawnConfig.Subnets,
		SecurityGroups: spawnConfig.SecurityGroups,
		AssignPublicIp: assignPublicIP,
	}
	networkConfiguration := ecstypes.NetworkConfiguration{
		AwsvpcConfiguration: &awsvpcConfiguration,
	}
	containerOverride := ecstypes.ContainerOverride{
		Name:        aws.String(spawnConfig.ContainerName),
		Environment: environment,
	}
	taskOverride := ecstypes.TaskOverride{
		ContainerOverrides: []ecstypes.ContainerOverride{containerOverride},
	}
	runTaskInput := ecs.RunTaskInput{
		Cluster:        aws.String(spawnConfig.Cluster),
		TaskDefinition: aws.String(spawnConfig.TaskDefinition),
		LaunchType:     ecstypes.LaunchTypeFargate,
		Count:          aws.Int32(1),
		StartedBy:      aws.String("agent-conductor"),
		// Required so local operators can attach with ECS Exec to the remote tmux session.
		EnableExecuteCommand: true,
		NetworkConfiguration: &networkConfiguration,
		Overrides:            &taskOverride,
		EnableECSManagedTags: true,
	}

	output, err := ecsClient.RunTask(ctx, &runTaskInput)
	if err != nil {
		return nil, fmt.Errorf("run Fargate task: %w", err)
	}
	if len(output.Failures) > 0 {
		return nil, fmt.Errorf("run Fargate task failed: %s", formatECSFailures(output.Failures))
	}
	if len(output.Tasks) != 1 {
		return nil, fmt.Errorf("expected one Fargate task, got %d", len(output.Tasks))
	}

	taskARN := aws.ToString(output.Tasks[0].TaskArn)
	if taskARN == "" {
		return nil, fmt.Errorf("ECS returned task without task ARN")
	}

	return &FargateAgentWorker{
		AgentJob:       agentJob,
		TriggerMessage: message,
		JobConfig:      agentConfig,
		TaskARN:        taskARN,
		Done:           make(chan struct{}),
		ecsClient:      ecsClient,
	}, nil
}

// spawnAndTrackAgentJob starts one Fargate Codex worker for a durable agent job.
// It persists spawn success/failure through the DB worker and starts the ECS monitor goroutine.
func spawnAndTrackAgentJob(ctx context.Context, databaseCommands chan<- DatabaseCommand, agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) bool {
	worker, err := spawnFargateAgent(ctx, agentJob, message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn Fargate agentJob=%d: %v\n", agentJob.ID, err)
		result := MarkAgentJobSpawnFailed(ctx, databaseCommands, agentJob.ID, err.Error())
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "mark agentJob spawn failed: %v\n", result.Err)
			return false
		}
		return true
	}

	spawned := MarkAgentJobSpawned(ctx, databaseCommands, agentJob.ID, worker.TaskARN)
	if spawned.Err != nil {
		fmt.Fprintf(os.Stderr, "mark agentJob spawned: %v\n", spawned.Err)
		return false
	}

	go worker.Run(ctx, databaseCommands)

	fmt.Printf("spawned Fargate agentJob=%d taskARN=%s\n", agentJob.ID, worker.TaskARN)
	return true
}

// buildAgentPrompt creates the initial Codex instruction string for a Fargate worker.
// It connects durable DB context to the wrapper's AGENT_PROMPT environment variable.
func buildAgentPrompt(agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) string {
	return fmt.Sprintf(
		"read agents.md and carry out the task. agent_job_id=%d message_id=%d message_type=%s raw_alert=%s",
		agentJob.ID,
		message.ID,
		message.MessageType,
		message.RawBody,
	)
}
