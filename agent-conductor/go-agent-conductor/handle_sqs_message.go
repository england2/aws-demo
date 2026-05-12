package main

import (
	"context"
	"fmt"

	"agent-orchestrator/fargate"
)

const defaultAgentName = "agent-fargate-codex"

type sqsMessageDeleter interface {
	DeleteMessage(ctx context.Context, receiptHandle string) error
}

var spawnFargateTask = fargate.Spawn

// HandleSQSMessage owns the complete no-state handling path for one SQS delivery.
func HandleSQSMessage(ctx context.Context, deleter sqsMessageDeleter, message SQSMessage) error {
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

func buildAgentPrompt(message SQSMessage) string {
	return fmt.Sprintf(
		"read agents.md and carry out the task.\n\nSQS message id: %s\n\nRaw SQS message body:\n%s",
		message.ExternalMessageID,
		message.RawBody,
	)
}
