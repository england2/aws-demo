//go:build !spawnfargate

package main

import (
	"context"
	"fmt"
	"os"

	"agent-orchestrator/fargate"
)

// main wires the conductor runtime from SQS polling into one Fargate spawn per received delivery.
// It creates the poller before starting the receive loop, then passes each parsed message to HandleSQSMessage
// so deletion remains ordered after a successful ECS RunTask call.

const defaultAgentName = "agent-fargate-codex"

type sqsMessageDeleter interface {
	DeleteMessage(ctx context.Context, receiptHandle string) error
}

// spawnFargateTask is the handler's replaceable edge into the fargate package.
// Production handling leaves it pointed at fargate.Spawn, while tests swap it before HandleSQSMessage runs
// so they can inspect the spawn request before the receipt-delete branch.
var spawnFargateTask = fargate.Spawn

// HandleSQSMessage is the single-message bridge from the SQS poller to the Fargate spawner.
// It builds the container environment from the already-received PolledSQSMessage, waits for Spawn to return a task ARN,
// and only then deletes the same receipt handle through the poller-provided deleter.
func HandleSQSMessage(ctx context.Context, deleter sqsMessageDeleter, message PolledSQSMessage) error {
	request := fargate.SpawnRequest{
		Config: adhocFargateSpawnConfig,
		Environment: fargate.WithDebugSSHEnvironment(map[string]string{
			"AGENT_NAME":   defaultAgentName,
			"AGENT_PROMPT": buildAgentPrompt(message),
		}),
	}

	result, err := spawnFargateTask(ctx, request)
	if err != nil {
		return fmt.Errorf("spawn fargate task: %w", err)
	}

	fmt.Printf("spawned Fargate task for sqsMessageID=%s taskARN=%s\n", message.ExternalMessageID, result.TaskARN)

	if err := deleter.DeleteMessage(ctx, message.ReceiptHandle); err != nil {
		return fmt.Errorf("delete spawned sqs message: %w", err)
	}

	return nil
}

// buildAgentPrompt turns the raw SQS delivery into the text passed as AGENT_PROMPT.
// HandleSQSMessage calls it after transport parsing has preserved Body, and the resulting string flows into
// fargate.SpawnRequest.Environment for the worker container wrapper to consume.
func buildAgentPrompt(message PolledSQSMessage) string {
	return fmt.Sprintf(
		"read agents.md and carry out the task.\n\nSQS message id: %s\n\nRaw SQS message body:\n%s",
		message.ExternalMessageID,
		message.Body,
	)
}

func main() {
	fmt.Println("Agent Conductor started!")

	ctx := context.Background()

	poller, err := NewTicketCloudWatchSQSPoller(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start sqs poller: %v\n", err)
		os.Exit(1)
	}
	messages, errors := poller.Start(ctx)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			if err := HandleSQSMessage(ctx, poller, message); err != nil {
				fmt.Fprintf(os.Stderr, "handle sqs message %s: %v\n", message.ExternalMessageID, err)
			}
		case err, ok := <-errors:
			if !ok {
				return
			}
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
