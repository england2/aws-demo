//go:build !spawnfargate

package main

import (
	"context"
	"fmt"
	"os"
)

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
