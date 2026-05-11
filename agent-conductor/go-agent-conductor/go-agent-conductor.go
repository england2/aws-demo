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

// main is the conductor entrypoint running on the debian-agent-operation EC2 host.
// It initializes SQLite, starts the DB worker, and processes inbound CloudWatch/ticket
// SQS messages into Fargate agent jobs.
func main() {

	fmt.Println("Agent Conductor started!")
	debugSSHEnabled, debugSSHPublicKeySecret := DebugSSHRuntimeEnv()
	fmt.Printf("debug ssh mode: enabled=%t publicKeySecret=%s\n", debugSSHEnabled, debugSSHPublicKeySecret)

	ctx := context.Background()

	// fail early if the conductor cannot find or initialize a usable database.
	if err := initializeRuntimeDatabase(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "load database: %v\n", err)
		os.Exit(1)
	}

	databaseCommands, err := StartDatabaseWorker(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start database worker: %v\n", err)
		os.Exit(1)
	}

	ticketCloudWatchSQSMessageQueue, errors := StartSQSPoller(ctx)

	for {
		select {
		case ticketCloudWatchSQSMessage, ok := <-ticketCloudWatchSQSMessageQueue:
			if !ok {
				return
			}
			result := ProcessInboundSQSMessageWithDatabase(ctx, databaseCommands, ticketCloudWatchSQSMessage)
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "process inbound sqs message: %v\n", result.Err)
				continue
			}

			fmt.Printf("inbound sqs message processed: reason=%s shouldSpawnAgentJob=%t messageID=%d\n", result.Reason, result.ShouldSpawnAgentJob, result.Message.ID)
			if result.AgentJob != nil {
				fmt.Printf("agentJob: id=%d agentName=%s spawnSQSMessageID=%d\n", result.AgentJob.ID, result.AgentJob.AgentName, result.AgentJob.SpawnSQSMessageID)
			}

			durableHandlingSucceeded := true
			if result.ShouldSpawnAgentJob && result.AgentJob != nil {
				durableHandlingSucceeded = spawnAndTrackAgentJob(ctx, databaseCommands, *result.AgentJob, result.Message)
			}

			if result.DeleteSQSMessage && durableHandlingSucceeded {
				if err := DeleteSQSMessage(ctx, ticketCloudWatchSQSMessage.ReceiptHandle); err != nil {
					fmt.Fprintf(os.Stderr, "delete sqs message: %v\n", err)
				}
			}
		case err, ok := <-errors:
			if !ok {
				return
			}
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// spawnAndTrackAgentJob starts one Fargate Codex worker for a durable agent job.
// It persists spawn success/failure through the DB worker and starts the ECS monitor goroutine.
func spawnAndTrackAgentJob(ctx context.Context, databaseCommands chan<- DatabaseCommand, agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) bool {
	markSpawnFailed := func(stage string, err error) bool {
		fmt.Fprintf(os.Stderr, "%s Fargate agentJob=%d: %v\n", stage, agentJob.ID, err)
		result := MarkAgentJobSpawnFailed(ctx, databaseCommands, agentJob.ID, err.Error())
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "mark agentJob spawn failed: %v\n", result.Err)
			return false
		}
		return true
	}

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
		return markSpawnFailed("configure", err)
	}

	awsConfig, err := loadFargateAWSConfig(ctx, spawnConfig.Region)
	if err != nil {
		return markSpawnFailed("load AWS config for", err)
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
		return markSpawnFailed("spawn", fmt.Errorf("run Fargate task: %w", err))
	}
	if len(output.Failures) > 0 {
		return markSpawnFailed("spawn", fmt.Errorf("run Fargate task failed: %s", formatECSFailures(output.Failures)))
	}
	if len(output.Tasks) != 1 {
		return markSpawnFailed("spawn", fmt.Errorf("expected one Fargate task, got %d", len(output.Tasks)))
	}

	taskARN := aws.ToString(output.Tasks[0].TaskArn)
	if taskARN == "" {
		return markSpawnFailed("spawn", fmt.Errorf("ECS returned task without task ARN"))
	}

	spawned := MarkAgentJobSpawned(ctx, databaseCommands, agentJob.ID, taskARN)
	if spawned.Err != nil {
		fmt.Fprintf(os.Stderr, "mark agentJob spawned: %v\n", spawned.Err)
		return false
	}

	worker := &FargateAgentWorker{
		AgentJob:         agentJob,
		TriggerMessage:   message,
		DatabaseCommands: databaseCommands,
		JobConfig:        agentConfig,
		TaskARN:          taskARN,
		Done:             make(chan struct{}),
		ecsClient:        ecsClient,
	}
	go worker.Run(ctx)

	fmt.Printf("spawned Fargate agentJob=%d taskARN=%s\n", agentJob.ID, taskARN)
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
