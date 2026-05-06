package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
)

// main is the conductor entrypoint running on the debian-agent-operation EC2 host.
// It initializes SQLite, starts the DB worker, starts the agent-event router, and
// processes inbound CloudWatch/ticket SQS messages into Fargate agent jobs.
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

	agentEventRouter, err := StartAgentEventRouter(ctx, databaseCommands, adhocAgentFargateEventsQueueURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start agent event router: %v\n", err)
		os.Exit(1)
	}
	go agentEventRouter.Run(ctx)

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
				durableHandlingSucceeded = spawnAndTrackAgentJob(ctx, databaseCommands, agentEventRouter, *result.AgentJob, result.Message)
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
// It persists spawn success/failure through the DB worker, registers the running-agent
// event channel with the router, and starts the ECS/event monitor goroutine.
func spawnAndTrackAgentJob(ctx context.Context, databaseCommands chan<- DatabaseCommand, router *AgentEventRouter, agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) bool {
	agentJobID := strconv.FormatInt(agentJob.ID, 10)
	debugSSHEnabled, debugSSHPublicKeySecret := DebugSSHRuntimeEnv()
	agentConfig := AgentFargateJobConfig{
		AWSFargateSpawnConfig: adhocAWSFargateSpawnConfig,
		RuntimeEnv: AgentFargateRuntimeEnv{
			AgentJobID:              agentJobID,
			AgentName:               agentJob.AgentName,
			Prompt:                  buildAgentPrompt(agentJob, message),
			EventsQueueURL:          adhocAgentFargateEventsQueueURL,
			DebugSSHEnabled:         debugSSHEnabled,
			DebugSSHPublicKeySecret: debugSSHPublicKeySecret,
		},
	}

	spawnResult, err := SpawnFargateAgent(ctx, agentConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn Fargate agentJob=%d: %v\n", agentJob.ID, err)
		result := MarkAgentJobSpawnFailed(ctx, databaseCommands, agentJob.ID, err.Error())
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "mark agentJob spawn failed: %v\n", result.Err)
			return false
		}
		return true
	}

	spawned := MarkAgentJobSpawned(ctx, databaseCommands, agentJob.ID, spawnResult.TaskARN)
	if spawned.Err != nil {
		fmt.Fprintf(os.Stderr, "mark agentJob spawned: %v\n", spawned.Err)
		return false
	}

	runningAgent, err := NewRunningFargateAgent(
		ctx,
		agentJobID,
		spawnResult.TaskARN,
		adhocAWSFargateSpawnConfig,
		adhocAgentFargateEventsQueueURL,
		databaseCommands,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create running Fargate agent manager: %v\n", err)
		result := MarkAgentJobSpawnFailed(ctx, databaseCommands, agentJob.ID, err.Error())
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "mark agentJob manager failed: %v\n", result.Err)
			return false
		}
		return true
	}

	router.Register <- RunningFargateAgentRegistration{
		AgentJobID: runningAgent.AgentJobID,
		Events:     runningAgent.AgentEvents,
	}

	go func() {
		runningAgent.Run(ctx)
		router.Unregister <- runningAgent.AgentJobID
	}()

	fmt.Printf("spawned Fargate agentJob=%d taskARN=%s\n", agentJob.ID, spawnResult.TaskARN)
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
