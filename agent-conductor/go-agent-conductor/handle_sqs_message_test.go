package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agent-orchestrator/fargate"
)

func TestHandleSQSMessageSpawnsAndDeletesReceiptHandle(t *testing.T) {
	originalSpawnFargateTask := spawnFargateTask
	t.Cleanup(func() {
		spawnFargateTask = originalSpawnFargateTask
	})

	var gotRequest fargate.SpawnRequest
	spawnFargateTask = func(ctx context.Context, request fargate.SpawnRequest) (fargate.SpawnResult, error) {
		gotRequest = request
		return fargate.SpawnResult{TaskARN: "arn:aws:ecs:us-west-2:123:task/42"}, nil
	}

	deleter := &fakeSQSMessageDeleter{}
	message := SQSMessage{
		ExternalMessageID: "message-1",
		ReceiptHandle:     "receipt-1",
		RawBody:           "investigate cpu alarm",
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

func TestHandleSQSMessageDoesNotDeleteWhenSpawnFails(t *testing.T) {
	originalSpawnFargateTask := spawnFargateTask
	t.Cleanup(func() {
		spawnFargateTask = originalSpawnFargateTask
	})

	spawnFargateTask = func(ctx context.Context, request fargate.SpawnRequest) (fargate.SpawnResult, error) {
		return fargate.SpawnResult{}, errors.New("boom")
	}

	deleter := &fakeSQSMessageDeleter{}
	err := HandleSQSMessage(context.Background(), deleter, SQSMessage{ReceiptHandle: "receipt-1"})
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

func (d *fakeSQSMessageDeleter) DeleteMessage(ctx context.Context, receiptHandle string) error {
	d.deletedReceiptHandle = receiptHandle
	return nil
}
