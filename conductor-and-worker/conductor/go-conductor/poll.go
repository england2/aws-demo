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

// PolledSQSMessage is one received SQS delivery plus the receipt handle needed to delete it.
type PolledSQSMessage struct {
	ExternalMessageID string
	ReceiptHandle     string
	Body              string
}

// Start launches the long-poll loop for a configured SQSPoller and exposes message/error channels to main.
// The poller must already have its AWS client and queue URL from NewSQSPoller; messages emitted here are later
// handed to main_testing, which owns scheduler insertion and delete ordering.
func (p *SQSPoller) Start(ctx context.Context) (<-chan PolledSQSMessage, <-chan error) {
	messages := make(chan PolledSQSMessage)
	errors := make(chan error)

	// This goroutine is the handoff from Start to the long-running Poll loop.
	// It owns channel closure after Poll exits, so main can treat closed message or error channels as shutdown.
	go func() {
		defer close(messages)
		defer close(errors)

		p.Poll(ctx, messages, errors)
	}()

	return messages, errors
}

// NewTicketCloudWatchSQSPoller builds the conductor's default inbound queue poller.
// It reads deploy-time env settings first, fills missing polling knobs with defaults, and delegates to
// NewSQSPoller before main starts the receive loop.
func NewTicketCloudWatchSQSPoller(ctx context.Context) (*SQSPoller, error) {
	return NewSQSPoller(ctx, SQSPollerConfig{
		QueueURL:                 getenvDefault("AGENT_OPERATION_QUEUE_URL", defaultQueueURL),
		WaitTimeSeconds:          int32FromEnv("AGENT_OPERATION_WAIT_TIME_SECONDS", 20),
		VisibilityTimeoutSeconds: int32FromEnv("AGENT_OPERATION_VISIBILITY_TIMEOUT_SECONDS", 60),
	})
}

// Poll continuously receives SQS messages, converts AWS transport values into PolledSQSMessage, and sends them onward.
// It runs after Start has created the channels, uses ReceiveMessages for each AWS call, and never deletes messages;
// deletion happens later after the scheduler accepts a supported message.
func (p *SQSPoller) Poll(ctx context.Context, messages chan<- PolledSQSMessage, errors chan<- error) {
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

// NewSQSPoller validates queue settings and constructs the AWS SQS client used by ReceiveMessages and DeleteMessage.
// It is called before any polling begins, so the queue URL, region, wait time, and visibility timeout are stable
// for the lifetime of the SQSPoller handed to main.
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

// ReceiveMessages performs one AWS long-poll request using this poller's established queue config.
// Poll calls it before parseSQSMessage, and the raw AWS messages returned here still carry the receipt handles
// needed later by DeleteMessage.
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

// DeleteMessage removes one SQS delivery from the same queue this poller receives from.
// main_testing calls it only after scheduler insertion succeeds, using the receipt handle that parseSQSMessage
// preserved from the current delivery.
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

// parseSQSMessage converts one AWS SDK message into the conductor's minimal PolledSQSMessage transport model.
// Poll calls it after ReceiveMessages and before sending work to main, preserving Body for scheduler RawBody
// and ReceiptHandle for the later successful-handling delete step.
func parseSQSMessage(message types.Message) (PolledSQSMessage, error) {
	if message.Body == nil {
		return PolledSQSMessage{}, fmt.Errorf("sqs message %s has empty body", aws.ToString(message.MessageId))
	}

	return PolledSQSMessage{
		ExternalMessageID: aws.ToString(message.MessageId),
		ReceiptHandle:     aws.ToString(message.ReceiptHandle),
		Body:              *message.Body,
	}, nil
}

// getenvDefault reads a string env override while preserving a hard-coded default for local and systemd runs.
// Queue setup calls it before creating AWS clients so local and deployed pollers use the same config path.
func getenvDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

// int32FromEnv reads numeric polling knobs from env and falls back on missing or malformed values.
// NewTicketCloudWatchSQSPoller calls it before NewSQSPoller validates config, keeping SQS wait and visibility
// settings deterministic for the poll loop.
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

// sendPollError forwards receive or parse errors from the polling goroutine to main's error branch.
// Poll calls it after failed AWS reads or message parsing, and the context check keeps shutdown from blocking
// on a receiver that has already exited.
func sendPollError(ctx context.Context, errors chan<- error, err error) {
	select {
	case errors <- err:
	case <-ctx.Done():
	}
}

// sleepUnlessCanceled pauses the Poll retry loop after transient AWS receive failures.
// It is used only after sendPollError has reported the failure, and it exits early when the conductor context
// has been canceled.
func sleepUnlessCanceled(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
