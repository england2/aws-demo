package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agent-orchestrator/fargate"
)

// TestHandleSQSMessageSpawnsAndDeletesReceiptHandle verifies the conductor's required SQS handling order.
// It replaces the Fargate spawn function before calling HandleSQSMessage, then checks that prompt/env data flows
// into the spawn request and the receipt handle is deleted only after a task ARN is returned.
func TestHandleSQSMessageSpawnsAndDeletesReceiptHandle(t *testing.T) {
	originalSpawnFargateTask := spawnFargateTask
	// This cleanup closure restores the production spawn hook after the test-specific fake is no longer needed.
	// It runs after HandleSQSMessage assertions so later tests and production code see the normal fargate.Spawn path.
	t.Cleanup(func() {
		spawnFargateTask = originalSpawnFargateTask
	})

	var gotRequest fargate.SpawnRequest
	// This fake spawn function captures the handler-built request and returns a task ARN without calling ECS.
	// HandleSQSMessage must complete this fake successfully before it is allowed to call the deleter.
	spawnFargateTask = func(ctx context.Context, request fargate.SpawnRequest) (fargate.SpawnResult, error) {
		gotRequest = request
		return fargate.SpawnResult{TaskARN: "arn:aws:ecs:us-west-2:123:task/42"}, nil
	}

	deleter := &fakeSQSMessageDeleter{}
	message := PolledSQSMessage{
		ExternalMessageID: "message-1",
		ReceiptHandle:     "receipt-1",
		Body:              "investigate cpu alarm",
	}

	if err := HandleSQSMessage(context.Background(), deleter, message); err != nil {
		t.Fatalf("HandleSQSMessage error: %v", err)
	}
	if deleter.deletedReceiptHandle != "receipt-1" {
		t.Fatalf("deleted receipt handle = %q", deleter.deletedReceiptHandle)
	}
	if gotRequest.Environment["AGENT_NAME"] != defaultAgentName {
		t.Fatalf("AGENT_NAME = %q", gotRequest.Environment["AGENT_NAME"])
	}
	if !strings.Contains(gotRequest.Environment["AGENT_PROMPT"], "investigate cpu alarm") {
		t.Fatalf("AGENT_PROMPT does not contain raw body: %q", gotRequest.Environment["AGENT_PROMPT"])
	}
}

// TestHandleSQSMessageDoesNotDeleteWhenSpawnFails verifies failed spawns leave the SQS delivery unacknowledged.
// It injects a failing spawn function before the handler runs, matching the production retry path where SQS can
// redeliver because DeleteMessage was never called.
func TestHandleSQSMessageDoesNotDeleteWhenSpawnFails(t *testing.T) {
	originalSpawnFargateTask := spawnFargateTask
	// This cleanup closure restores the package-level spawn hook after the failure path is exercised.
	// It preserves test isolation because HandleSQSMessage reads spawnFargateTask at call time.
	t.Cleanup(func() {
		spawnFargateTask = originalSpawnFargateTask
	})

	// This fake spawn function forces the handler to stop before SQS acknowledgement.
	// HandleSQSMessage receives the error at the same point production code would receive an ECS spawn failure.
	spawnFargateTask = func(ctx context.Context, request fargate.SpawnRequest) (fargate.SpawnResult, error) {
		return fargate.SpawnResult{}, errors.New("boom")
	}

	deleter := &fakeSQSMessageDeleter{}
	err := HandleSQSMessage(context.Background(), deleter, PolledSQSMessage{ReceiptHandle: "receipt-1"})
	if err == nil {
		t.Fatalf("HandleSQSMessage error = nil, want error")
	}
	if deleter.deletedReceiptHandle != "" {
		t.Fatalf("deleted receipt handle = %q, want none", deleter.deletedReceiptHandle)
	}
}

type fakeSQSMessageDeleter struct {
	deletedReceiptHandle string
}

// DeleteMessage records the receipt handle passed by HandleSQSMessage instead of calling AWS SQS.
// Tests use it after fake spawn completion to confirm the handler deletes the exact delivery it received.
func (d *fakeSQSMessageDeleter) DeleteMessage(ctx context.Context, receiptHandle string) error {
	d.deletedReceiptHandle = receiptHandle
	return nil
}
