package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// FargateAgentWorker is the server-side owner for one starting/running Fargate
// agent. spawnFargateAgent builds it, and Run monitors ECS status while
// database writes stay in the caller-provided command channel.
type FargateAgentWorker struct {
	AgentJob       DatabaseAgentJobInfo
	TriggerMessage DatabaseSQSMessageInfo

	JobConfig AgentFargateJobConfig
	TaskARN   string
	Done      chan struct{}

	ecsClient ecsFargateClient
}

type ecsFargateClient interface {
	RunTask(ctx context.Context, params *ecs.RunTaskInput, optFns ...func(*ecs.Options)) (*ecs.RunTaskOutput, error)
	DescribeTasks(ctx context.Context, params *ecs.DescribeTasksInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

// Run is the per-agent control loop for ECS task observations.
// It exits when ECS reports STOPPED, the database reports terminal state, or context ends.
func (worker *FargateAgentWorker) Run(ctx context.Context, databaseCommands chan<- DatabaseCommand) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	defer close(worker.Done)

	for {
		select {
		case <-ticker.C:
			if worker.PollECSStatus(ctx, databaseCommands) {
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
func (worker *FargateAgentWorker) PollECSStatus(ctx context.Context, databaseCommands chan<- DatabaseCommand) bool {
	output, err := worker.ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(worker.JobConfig.AWSFargateSpawnConfig.Cluster),
		Tasks:   []string{worker.TaskARN},
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
		agentJobID, err := worker.parseAgentJobIDForDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse agent job id for stopped Fargate task: %v\n", err)
			return false
		}
		result := MarkAgentJobTaskStopped(ctx, databaseCommands, agentJobID, lastStatus, stoppedReason)
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "mark stopped Fargate agent job: %v\n", result.Err)
			return false
		}
		return true
	}

	agentJobID, err := worker.parseAgentJobIDForDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse agent job id for ECS status: %v\n", err)
		return false
	}
	result := UpdateAgentJobECSStatus(ctx, databaseCommands, agentJobID, lastStatus, stoppedReason)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "update Fargate ECS status: %v\n", result.Err)
		return false
	}

	return result.Terminal
}

// parseAgentJobIDForDB converts the worker runtime job ID into a DB ID for ECS status updates.
func (worker *FargateAgentWorker) parseAgentJobIDForDB() (int64, error) {
	agentJobID := worker.JobConfig.RuntimeEnv.AgentJobID
	parsed, err := strconv.ParseInt(agentJobID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse agent job id %q: %w", agentJobID, err)
	}

	return parsed, nil
}
