package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"agentproto"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type AgentEventEnvelope struct {
	Event             agentproto.AgentEvent
	ReceiptHandle     string
	RawBody           string
	ExternalMessageID string
}

type RunningFargateAgent struct {
	AgentJobID string
	TaskARN    string

	AWSFargateSpawnConfig AWSFargateSpawnConfig
	EventsQueueURL        string
	DatabaseCommands      chan<- DatabaseCommand
	AgentEvents           chan AgentEventEnvelope
	Done                  chan struct{}

	ecsClient               *ecs.Client
	deleteAgentEventMessage func(context.Context, string) error
}

type AgentEventRouter struct {
	client           *sqs.Client
	queueURL         string
	DatabaseCommands chan<- DatabaseCommand
	Agents           map[string]chan<- AgentEventEnvelope
	Register         chan RunningFargateAgentRegistration
	Unregister       chan string
}

type RunningFargateAgentRegistration struct {
	AgentJobID string
	Events     chan<- AgentEventEnvelope
}

func StartAgentEventRouter(ctx context.Context, databaseCommands chan<- DatabaseCommand, queueURL string) (*AgentEventRouter, error) {
	if queueURL == "" {
		return nil, fmt.Errorf("agent event queue URL is required")
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(getenvDefault("AWS_REGION", "us-west-2")))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for agent event router: %w", err)
	}

	return &AgentEventRouter{
		client:           sqs.NewFromConfig(awsConfig),
		queueURL:         queueURL,
		DatabaseCommands: databaseCommands,
		Agents:           make(map[string]chan<- AgentEventEnvelope),
		Register:         make(chan RunningFargateAgentRegistration),
		Unregister:       make(chan string),
	}, nil
}

func (router *AgentEventRouter) Run(ctx context.Context) {
	messages := make(chan AgentEventEnvelope)
	errors := make(chan error)
	go router.poll(ctx, messages, errors)

	for {
		select {
		case <-ctx.Done():
			return
		case registration := <-router.Register:
			router.Agents[registration.AgentJobID] = registration.Events
			fmt.Printf("registered running Fargate agentJob=%s\n", registration.AgentJobID)
		case agentJobID := <-router.Unregister:
			delete(router.Agents, agentJobID)
			fmt.Printf("unregistered running Fargate agentJob=%s\n", agentJobID)
		case envelope := <-messages:
			router.routeEnvelope(ctx, envelope)
		case err := <-errors:
			fmt.Fprintf(os.Stderr, "agent event router: %v\n", err)
		}
	}
}

func (router *AgentEventRouter) routeEnvelope(ctx context.Context, envelope AgentEventEnvelope) {
	events, ok := router.Agents[envelope.Event.JobID]
	if ok {
		select {
		case events <- envelope:
		case <-ctx.Done():
		}
		return
	}

	result := RecordAgentEventWithDatabase(ctx, router.DatabaseCommands, envelope.Event, envelope.RawBody)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "record unrouted agent event: %v\n", result.Err)
		return
	}

	if err := router.DeleteMessage(ctx, envelope.ReceiptHandle); err != nil {
		fmt.Fprintf(os.Stderr, "delete unrouted agent event: %v\n", err)
	}
}

func (router *AgentEventRouter) poll(ctx context.Context, messages chan<- AgentEventEnvelope, errors chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		output, err := router.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:                    aws.String(router.queueURL),
			MaxNumberOfMessages:         1,
			WaitTimeSeconds:             20,
			VisibilityTimeout:           60,
			MessageAttributeNames:       []string{"All"},
			MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{sqstypes.MessageSystemAttributeNameAll},
		})
		if err != nil {
			sendAgentEventRouterError(ctx, errors, fmt.Errorf("receive agent event message: %w", err))
			sleepUnlessCanceled(ctx, 5*time.Second)
			continue
		}

		for _, message := range output.Messages {
			envelope, err := ParseAgentEventSQSMessage(message)
			if err != nil {
				sendAgentEventRouterError(ctx, errors, err)
				continue
			}

			select {
			case messages <- envelope:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (router *AgentEventRouter) DeleteMessage(ctx context.Context, receiptHandle string) error {
	if receiptHandle == "" {
		return fmt.Errorf("agent event receipt handle is required")
	}

	_, err := router.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(router.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete agent event message: %w", err)
	}

	return nil
}

func ParseAgentEventSQSMessage(message sqstypes.Message) (AgentEventEnvelope, error) {
	if message.Body == nil {
		return AgentEventEnvelope{}, fmt.Errorf("agent event SQS message %s has empty body", aws.ToString(message.MessageId))
	}

	var event agentproto.AgentEvent
	if err := json.Unmarshal([]byte(*message.Body), &event); err != nil {
		return AgentEventEnvelope{}, fmt.Errorf("parse agent event SQS message %s: %w", aws.ToString(message.MessageId), err)
	}

	return AgentEventEnvelope{
		Event:             event,
		ReceiptHandle:     aws.ToString(message.ReceiptHandle),
		RawBody:           *message.Body,
		ExternalMessageID: aws.ToString(message.MessageId),
	}, nil
}

func NewRunningFargateAgent(ctx context.Context, agentJobID string, taskARN string, spawnConfig AWSFargateSpawnConfig, eventsQueueURL string, databaseCommands chan<- DatabaseCommand, deleteAgentEventMessage func(context.Context, string) error) (*RunningFargateAgent, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(spawnConfig.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for running Fargate agent: %w", err)
	}

	return &RunningFargateAgent{
		AgentJobID:              agentJobID,
		TaskARN:                 taskARN,
		AWSFargateSpawnConfig:   spawnConfig,
		EventsQueueURL:          eventsQueueURL,
		DatabaseCommands:        databaseCommands,
		AgentEvents:             make(chan AgentEventEnvelope, 32),
		Done:                    make(chan struct{}),
		ecsClient:               ecs.NewFromConfig(awsConfig),
		deleteAgentEventMessage: deleteAgentEventMessage,
	}, nil
}

func (agent *RunningFargateAgent) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	defer close(agent.Done)

	for {
		select {
		case envelope := <-agent.AgentEvents:
			if agent.RespondAgentEvent(ctx, envelope) {
				return
			}
		case <-ticker.C:
			if agent.PollECSStatus(ctx) {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (agent *RunningFargateAgent) RespondAgentEvent(ctx context.Context, envelope AgentEventEnvelope) bool {
	fmt.Printf("agent event: jobID=%s type=%s eventID=%s\n", envelope.Event.JobID, envelope.Event.Type, envelope.Event.EventID)

	result := RecordAgentEventWithDatabase(ctx, agent.DatabaseCommands, envelope.Event, envelope.RawBody)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "record agent event: %v\n", result.Err)
		return false
	}

	if err := agent.deleteAgentEventMessage(ctx, envelope.ReceiptHandle); err != nil {
		fmt.Fprintf(os.Stderr, "delete agent event message: %v\n", err)
	}

	switch envelope.Event.Type {
	case agentproto.AgentWrapperStarted:
	case agentproto.AgentSetupStarted:
	case agentproto.AgentSetupFailed:
	case agentproto.CodexStarted:
	case agentproto.CodexExited:
	case agentproto.AgentReportedSuccess:
	case agentproto.AgentReportedFailure:
	case agentproto.PullRequestCreated:
	case agentproto.JobCompleted:
	case agentproto.JobFailed:
	}

	return result.Terminal
}

func (agent *RunningFargateAgent) PollECSStatus(ctx context.Context) bool {
	output, err := agent.ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(agent.AWSFargateSpawnConfig.Cluster),
		Tasks:   []string{agent.TaskARN},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "describe Fargate task: %v\n", err)
		return false
	}
	if len(output.Tasks) != 1 {
		fmt.Fprintf(os.Stderr, "describe Fargate task: expected one task, got %d\n", len(output.Tasks))
		return false
	}

	task := output.Tasks[0]
	lastStatus := aws.ToString(task.LastStatus)
	stoppedReason := aws.ToString(task.StoppedReason)

	if lastStatus == "STOPPED" {
		agentJobID, err := parseAgentJobIDForDB(agent.AgentJobID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse agent job id for stopped Fargate task: %v\n", err)
			return false
		}
		result := MarkAgentJobTaskStopped(ctx, agent.DatabaseCommands, agentJobID, lastStatus, stoppedReason)
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "mark stopped Fargate agent job: %v\n", result.Err)
			return false
		}
		return true
	}

	agentJobID, err := parseAgentJobIDForDB(agent.AgentJobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse agent job id for ECS status: %v\n", err)
		return false
	}
	result := UpdateAgentJobECSStatus(ctx, agent.DatabaseCommands, agentJobID, lastStatus, stoppedReason)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "update Fargate ECS status: %v\n", result.Err)
		return false
	}

	return result.Terminal
}

func sendAgentEventRouterError(ctx context.Context, errors chan<- error, err error) {
	select {
	case errors <- err:
	case <-ctx.Done():
	}
}

func parseAgentJobIDForDB(agentJobID string) (int64, error) {
	parsed, err := strconv.ParseInt(agentJobID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse agent job id %q: %w", agentJobID, err)
	}

	return parsed, nil
}
