package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// RunningFargateAgent is the in-memory controller for one spawned ECS task.
// It links the durable agent job ID, ECS task ARN, DB command channel, and ECS client
// used to monitor that specific remote worker.
type RunningFargateAgent struct {
	AgentJobID string
	TaskARN    string

	AWSFargateSpawnConfig AWSFargateSpawnConfig
	DatabaseCommands      chan<- DatabaseCommand
	Done                  chan struct{}

	ecsClient *ecs.Client
}

// NewRunningFargateAgent builds the per-task monitor/controller after ECS accepts a task.
// It creates the ECS client used for DescribeTasks.
func NewRunningFargateAgent(ctx context.Context, agentJobID string, taskARN string, spawnConfig AWSFargateSpawnConfig, databaseCommands chan<- DatabaseCommand) (*RunningFargateAgent, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(spawnConfig.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for running Fargate agent: %w", err)
	}

	return &RunningFargateAgent{
		AgentJobID:            agentJobID,
		TaskARN:               taskARN,
		AWSFargateSpawnConfig: spawnConfig,
		DatabaseCommands:      databaseCommands,
		Done:                  make(chan struct{}),
		ecsClient:             ecs.NewFromConfig(awsConfig),
	}, nil
}

// Run is the per-agent control loop for ECS task observations.
// It exits when ECS reports STOPPED, the database reports terminal state, or context ends.
func (agent *RunningFargateAgent) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	defer close(agent.Done)

	for {
		select {
		case <-ticker.C:
			if agent.PollECSStatus(ctx) {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// PollECSStatus observes ECS task state and writes status back through the DB worker.
// STOPPED is treated as a failed job so jobs do not remain stuck running when the
// container crashes or exits unexpectedly.
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

// parseAgentJobIDForDB converts string job IDs into DB IDs for ECS status updates.
func parseAgentJobIDForDB(agentJobID string) (int64, error) {
	parsed, err := strconv.ParseInt(agentJobID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse agent job id %q: %w", agentJobID, err)
	}

	return parsed, nil
}
