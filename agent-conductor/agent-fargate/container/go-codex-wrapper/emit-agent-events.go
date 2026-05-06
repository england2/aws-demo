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

// AgentEventEmitter sends wrapper lifecycle events to the conductor-owned SQS queue.
// It connects the Fargate-side wrapper to the EC2 conductor's agent-event router.
// The emitter depends on AWS SDK credentials from the Fargate task role and on
// AGENT_FARGATE_EVENTS_QUEUE_URL, AGENT_JOB_ID, AGENT_NAME, and AWS_REGION env vars.
type AgentEventEmitter struct {
	client    *sqs.Client
	queueURL  string
	jobID     string
	agentName string
}

// NewAgentEventEmitter builds the SQS-backed event emitter used by the wrapper.
// It validates the runtime environment injected by the conductor before Codex starts.
// This fails early when the task cannot report status back to the conductor.
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

// Send emits one agentproto.AgentEvent to the shared Fargate-events SQS queue.
// The conductor correlates this message by JobID and uses EventID for durable
// de-duplication once event persistence is implemented. Each send creates a new
// logical event with a fresh UUID and current UTC timestamp.
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

// SendFailure is the best-effort error reporting path for setup/runtime failures.
// It intentionally never returns an error because callers are already handling a
// primary failure path and should not mask that failure with telemetry problems.
func (emitter *AgentEventEmitter) SendFailure(ctx context.Context, eventType agentproto.AgentEventType, err error) {
	if emitter == nil || err == nil {
		return
	}

	if sendErr := emitter.Send(ctx, eventType, err.Error()); sendErr != nil {
		fmt.Fprintf(os.Stderr, "send failure agent event: %v\n", sendErr)
	}
}
