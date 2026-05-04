package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
)

func main() {

	fmt.Println("Agent Conductor started!")

	// fail early if the conductor cannot find or initialize a usable database.
	if err := check_load_db(); err != nil {
		fmt.Fprintf(os.Stderr, "load database: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
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

	messages, errors := StartSQSPoller(ctx)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			result := RecordSQSMessageWithDatabase(ctx, databaseCommands, message)
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "record sqs message: %v\n", result.Err)
				continue
			}

			fmt.Printf("database decision: reason=%s shouldSpawnAgentJob=%t messageID=%d\n", result.Reason, result.ShouldSpawnAgentJob, result.Message.ID)
			if result.AgentJob != nil {
				fmt.Printf("agentJob: id=%d agentName=%s spawnSQSMessageID=%d\n", result.AgentJob.ID, result.AgentJob.AgentName, result.AgentJob.SpawnSQSMessageID)
			}

			durableHandlingSucceeded := true
			if result.ShouldSpawnAgentJob && result.AgentJob != nil {
				durableHandlingSucceeded = spawnAndTrackAgentJob(ctx, databaseCommands, agentEventRouter, *result.AgentJob, result.Message)
			}

			if result.DeleteSQSMessage && durableHandlingSucceeded {
				if err := DeleteSQSMessage(ctx, message.ReceiptHandle); err != nil {
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

func spawnAndTrackAgentJob(ctx context.Context, databaseCommands chan<- DatabaseCommand, router *AgentEventRouter, agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) bool {
	agentJobID := strconv.FormatInt(agentJob.ID, 10)
	agentConfig := AgentFargateJobConfig{
		AWSFargateSpawnConfig: adhocAWSFargateSpawnConfig,
		RuntimeEnv: AgentFargateRuntimeEnv{
			AgentJobID:     agentJobID,
			AgentName:      agentJob.AgentName,
			Prompt:         buildAgentPrompt(agentJob, message),
			EventsQueueURL: adhocAgentFargateEventsQueueURL,
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
		router.DeleteMessage,
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

func buildAgentPrompt(agentJob DatabaseAgentJobInfo, message DatabaseSQSMessageInfo) string {
	return fmt.Sprintf(
		"read agents.md and carry out the task. agent_job_id=%d message_id=%d message_type=%s raw_alert=%s",
		agentJob.ID,
		message.ID,
		message.MessageType,
		message.RawBody,
	)
}
