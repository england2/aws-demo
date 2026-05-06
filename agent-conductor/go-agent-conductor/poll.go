package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const defaultQueueURL = "https://sqs.us-west-2.amazonaws.com/204772699175/agent-operation-events"

// SQSPoller owns AWS SQS client state for one queue.
// It is shared by ticket/CloudWatch input polling and agent-event queue polling.
type SQSPoller struct {
	client                   *sqs.Client
	queueURL                 string
	waitTimeSeconds          int32
	visibilityTimeoutSeconds int32
}

// SQSPollerConfig describes one SQS long-polling connection.
// Callers provide queue URL and optional runtime tuning values; defaults are filled in NewSQSPoller.
type SQSPollerConfig struct {
	QueueURL                 string
	Region                   string
	WaitTimeSeconds          int32
	VisibilityTimeoutSeconds int32
}

// StartSQSPoller starts the primary ticket/CloudWatch input queue consumer.
// It returns channels so main can process one parsed SQS delivery at a time.
func StartSQSPoller(ctx context.Context) (<-chan DatabaseSQSMessageInfo, <-chan error) {
	messages := make(chan DatabaseSQSMessageInfo)
	errors := make(chan error)

	go func() {
		defer close(messages)
		defer close(errors)

		poller, err := NewTicketCloudWatchSQSPoller(ctx)
		if err != nil {
			sendPollError(ctx, errors, err)
			return
		}

		poller.Poll(ctx, messages, errors)
	}()

	return messages, errors
}

// DeleteSQSMessage deletes one primary input queue delivery by receipt handle.
// Main calls this only after durable database handling and any spawn attempt are complete.
func DeleteSQSMessage(ctx context.Context, receiptHandle string) error {
	poller, err := NewTicketCloudWatchSQSPoller(ctx)
	if err != nil {
		return err
	}

	return poller.DeleteMessage(ctx, receiptHandle)
}

// NewTicketCloudWatchSQSPoller builds the poller for the conductor's inbound work queue.
// It reads queue URL and timing config from env with hard-coded demo defaults.
func NewTicketCloudWatchSQSPoller(ctx context.Context) (*SQSPoller, error) {
	return NewSQSPoller(ctx, SQSPollerConfig{
		QueueURL:                 getenvDefault("AGENT_OPERATION_QUEUE_URL", defaultQueueURL),
		WaitTimeSeconds:          int32FromEnv("AGENT_OPERATION_WAIT_TIME_SECONDS", 20),
		VisibilityTimeoutSeconds: int32FromEnv("AGENT_OPERATION_VISIBILITY_TIMEOUT_SECONDS", 60),
	})
}

// Poll continuously long-polls SQS and emits parsed database message structs.
// It intentionally receives one SQS message at a time to keep the conductor's
// database-first intake and agent spawning flow simple and serialized.
func (p *SQSPoller) Poll(ctx context.Context, messages chan<- DatabaseSQSMessageInfo, errors chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		rawMessages, err := p.ReceiveMessages(ctx)
		if err != nil {
			sendPollError(ctx, errors, err)
			sleepUnlessCanceled(ctx, 5*time.Second)
			continue
		}

		for _, message := range rawMessages {
			sqsMessage, err := parseSQSMessage(message)
			if err != nil {
				sendPollError(ctx, errors, err)
				continue
			}

			select {
			case messages <- sqsMessage:
			case <-ctx.Done():
				return
			}
		}
	}
}

// NewSQSPoller creates an AWS SQS client and validates poller settings.
// It depends on standard AWS SDK credential loading, normally the EC2 instance role.
func NewSQSPoller(ctx context.Context, pollerConfig SQSPollerConfig) (*SQSPoller, error) {
	if pollerConfig.QueueURL == "" {
		return nil, fmt.Errorf("SQS queue URL is required")
	}
	if pollerConfig.Region == "" {
		pollerConfig.Region = getenvDefault("AWS_REGION", "us-west-2")
	}
	if pollerConfig.WaitTimeSeconds == 0 {
		pollerConfig.WaitTimeSeconds = 20
	}
	if pollerConfig.VisibilityTimeoutSeconds == 0 {
		pollerConfig.VisibilityTimeoutSeconds = 60
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(pollerConfig.Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &SQSPoller{
		client:                   sqs.NewFromConfig(awsConfig),
		queueURL:                 pollerConfig.QueueURL,
		waitTimeSeconds:          pollerConfig.WaitTimeSeconds,
		visibilityTimeoutSeconds: pollerConfig.VisibilityTimeoutSeconds,
	}, nil
}

// ReceiveMessages performs one SQS long-poll request using this poller's queue config.
// It returns raw AWS messages so specific queue consumers can parse their own body shape.
func (p *SQSPoller) ReceiveMessages(ctx context.Context) ([]types.Message, error) {
	output, err := p.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(p.queueURL),
		// Only deal with one message at a time.
		MaxNumberOfMessages:         1,
		WaitTimeSeconds:             p.waitTimeSeconds,
		VisibilityTimeout:           p.visibilityTimeoutSeconds,
		MessageAttributeNames:       []string{"All"},
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{types.MessageSystemAttributeNameAll},
	})
	if err != nil {
		return nil, fmt.Errorf("receive sqs message: %w", err)
	}

	return output.Messages, nil
}

// DeleteMessage removes a successfully handled SQS delivery from this poller's queue.
// The receipt handle must come from the current delivery, not from an earlier redelivery.
func (p *SQSPoller) DeleteMessage(ctx context.Context, receiptHandle string) error {
	if receiptHandle == "" {
		return fmt.Errorf("receipt handle is required")
	}

	_, err := p.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(p.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete sqs message: %w", err)
	}

	return nil
}

// parseSQSMessage converts AWS transport data into the conductor's raw message model.
// Business parsing happens later in the database intake path so raw bodies are preserved.
func parseSQSMessage(message types.Message) (DatabaseSQSMessageInfo, error) {
	if message.Body == nil {
		return DatabaseSQSMessageInfo{}, fmt.Errorf("sqs message %s has empty body", aws.ToString(message.MessageId))
	}

	return DatabaseSQSMessageInfo{
		ExternalMessageID: aws.ToString(message.MessageId),
		ReceiptHandle:     aws.ToString(message.ReceiptHandle),
		Body:              []byte(*message.Body),
		RawBody:           *message.Body,
	}, nil
}

// getenvDefault reads an environment variable with a fallback.
// The conductor uses this for simple deploy-time config without a full config system.
func getenvDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

// int32FromEnv reads an int32 env setting with a safe fallback on missing/bad values.
// This keeps polling knobs robust when running from systemd or SSM-provided env.
func int32FromEnv(name string, fallback int32) int32 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fallback
	}

	return int32(parsed)
}

// sendPollError forwards poller errors unless the conductor context is canceled.
// Polling goroutines use this instead of blocking forever on shutdown.
func sendPollError(ctx context.Context, errors chan<- error, err error) {
	select {
	case errors <- err:
	case <-ctx.Done():
	}
}

// sleepUnlessCanceled backs off a polling loop while still responding to shutdown.
// It avoids busy retrying on AWS/SQS errors.
func sleepUnlessCanceled(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
