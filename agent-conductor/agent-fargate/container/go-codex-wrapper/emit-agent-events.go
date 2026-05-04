package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"agentproto"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
)

type AgentEventEmitter struct {
	client    *sqs.Client
	queueURL  string
	jobID     string
	agentName string
}

func NewAgentEventEmitter(ctx context.Context) (*AgentEventEmitter, error) {
	queueURL := strings.TrimSpace(os.Getenv("AGENT_FARGATE_EVENTS_QUEUE_URL"))
	if queueURL == "" {
		return nil, fmt.Errorf("AGENT_FARGATE_EVENTS_QUEUE_URL is required")
	}

	jobID := strings.TrimSpace(os.Getenv("AGENT_JOB_ID"))
	if jobID == "" {
		return nil, fmt.Errorf("AGENT_JOB_ID is required")
	}

	agentName := strings.TrimSpace(os.Getenv("AGENT_NAME"))
	if agentName == "" {
		agentName = "agent-fargate-codex"
	}

	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = "us-west-2"
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for agent event emitter: %w", err)
	}

	return &AgentEventEmitter{
		client:    sqs.NewFromConfig(awsConfig),
		queueURL:  queueURL,
		jobID:     jobID,
		agentName: agentName,
	}, nil
}

func (emitter *AgentEventEmitter) Send(ctx context.Context, eventType agentproto.AgentEventType, message string) error {
	event := agentproto.AgentEvent{
		EventID:   uuid.NewString(),
		JobID:     emitter.jobID,
		AgentName: emitter.agentName,
		Type:      eventType,
		Message:   message,
		CreatedAt: time.Now().UTC(),
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal agent event: %w", err)
	}

	_, err = emitter.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(emitter.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("send agent event %s: %w", eventType, err)
	}

	return nil
}

func (emitter *AgentEventEmitter) SendFailure(ctx context.Context, eventType agentproto.AgentEventType, err error) {
	if emitter == nil || err == nil {
		return
	}

	if sendErr := emitter.Send(ctx, eventType, err.Error()); sendErr != nil {
		fmt.Fprintf(os.Stderr, "send failure agent event: %v\n", sendErr)
	}
}
