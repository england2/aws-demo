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

	"agent-orchestrator/agent-spawners/lib"
)

const defaultQueueURL = "https://sqs.us-west-2.amazonaws.com/204772699175/agent-operation-events"

type SQSPoller struct {
	client                   *sqs.Client
	queueURL                 string
	waitTimeSeconds          int32
	visibilityTimeoutSeconds int32
}

func StartSQSPoller(ctx context.Context) (<-chan lib.SQSMessage, <-chan error) {
	messages := make(chan lib.SQSMessage)
	errors := make(chan error)

	go func() {
		defer close(messages)
		defer close(errors)

		poller, err := NewSQSPoller(ctx)
		if err != nil {
			sendPollError(ctx, errors, err)
			return
		}

		poller.Poll(ctx, messages, errors)
	}()

	return messages, errors
}

func DeleteSQSMessage(ctx context.Context, receiptHandle string) error {
	poller, err := NewSQSPoller(ctx)
	if err != nil {
		return err
	}

	return poller.DeleteMessage(ctx, receiptHandle)
}

func NewSQSPoller(ctx context.Context) (*SQSPoller, error) {
	region := getenvDefault("AWS_REGION", "us-west-2")

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &SQSPoller{
		client:                   sqs.NewFromConfig(awsConfig),
		queueURL:                 getenvDefault("AGENT_OPERATION_QUEUE_URL", defaultQueueURL),
		waitTimeSeconds:          int32FromEnv("AGENT_OPERATION_WAIT_TIME_SECONDS", 20),
		visibilityTimeoutSeconds: int32FromEnv("AGENT_OPERATION_VISIBILITY_TIMEOUT_SECONDS", 60),
	}, nil
}

func (p *SQSPoller) Poll(ctx context.Context, messages chan<- lib.SQSMessage, errors chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		output, err := p.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl: aws.String(p.queueURL),
			// This avoids one receive call triggering a burst of agents before incident-chain
			// coordination has a chance to decide whether alerts belong to the same incident.
			MaxNumberOfMessages:         1,
			WaitTimeSeconds:             p.waitTimeSeconds,
			VisibilityTimeout:           p.visibilityTimeoutSeconds,
			MessageAttributeNames:       []string{"All"},
			MessageSystemAttributeNames: []types.MessageSystemAttributeName{types.MessageSystemAttributeNameAll},
		})
		if err != nil {
			sendPollError(ctx, errors, fmt.Errorf("receive sqs message: %w", err))
			sleepUnlessCanceled(ctx, 5*time.Second)
			continue
		}

		for _, message := range output.Messages {
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

func parseSQSMessage(message types.Message) (lib.SQSMessage, error) {
	if message.Body == nil {
		return lib.SQSMessage{}, fmt.Errorf("sqs message %s has empty body", aws.ToString(message.MessageId))
	}

	return lib.SQSMessage{
		MessageID:     aws.ToString(message.MessageId),
		ReceiptHandle: aws.ToString(message.ReceiptHandle),
		Body:          []byte(*message.Body),
		RawBody:       *message.Body,
	}, nil
}

func getenvDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

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

func sendPollError(ctx context.Context, errors chan<- error, err error) {
	select {
	case errors <- err:
	case <-ctx.Done():
	}
}

func sleepUnlessCanceled(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
