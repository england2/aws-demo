package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type AWSFargateSpawnConfig struct {
	Region         string
	Cluster        string
	TaskDefinition string
	ContainerName  string
	Subnets        []string
	SecurityGroups []string
	AssignPublicIP bool
}

type AgentFargateJobConfig struct {
	AWSFargateSpawnConfig AWSFargateSpawnConfig

	AgentJobID     string
	AgentName      string
	Prompt         string
	EventsQueueURL string
}

type AgentFargateSpawnResult struct {
	TaskARN string
}

func SpawnFargateAgent(ctx context.Context, agentConfig AgentFargateJobConfig) (AgentFargateSpawnResult, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(agentConfig.AWSFargateSpawnConfig.Region))
	if err != nil {
		return AgentFargateSpawnResult{}, fmt.Errorf("load AWS config: %w", err)
	}

	input, err := BuildRunTaskInput(agentConfig)
	if err != nil {
		return AgentFargateSpawnResult{}, err
	}

	output, err := ecs.NewFromConfig(awsConfig).RunTask(ctx, input)
	if err != nil {
		return AgentFargateSpawnResult{}, fmt.Errorf("run Fargate task: %w", err)
	}
	if len(output.Failures) > 0 {
		return AgentFargateSpawnResult{}, fmt.Errorf("run Fargate task failed: %s", formatECSFailures(output.Failures))
	}
	if len(output.Tasks) != 1 {
		return AgentFargateSpawnResult{}, fmt.Errorf("expected one Fargate task, got %d", len(output.Tasks))
	}

	taskARN := aws.ToString(output.Tasks[0].TaskArn)
	if taskARN == "" {
		return AgentFargateSpawnResult{}, fmt.Errorf("ECS returned task without task ARN")
	}

	return AgentFargateSpawnResult{TaskARN: taskARN}, nil
}

func BuildRunTaskInput(agentConfig AgentFargateJobConfig) (*ecs.RunTaskInput, error) {
	if err := validateAgentFargateJobConfig(agentConfig); err != nil {
		return nil, err
	}

	assignPublicIP := ecstypes.AssignPublicIpDisabled
	if agentConfig.AWSFargateSpawnConfig.AssignPublicIP {
		assignPublicIP = ecstypes.AssignPublicIpEnabled
	}

	return &ecs.RunTaskInput{
		Cluster:        aws.String(agentConfig.AWSFargateSpawnConfig.Cluster),
		TaskDefinition: aws.String(agentConfig.AWSFargateSpawnConfig.TaskDefinition),
		LaunchType:     ecstypes.LaunchTypeFargate,
		Count:          aws.Int32(1),
		ClientToken:    aws.String("agent-job-" + agentConfig.AgentJobID),
		StartedBy:      aws.String("agent-conductor"),
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        agentConfig.AWSFargateSpawnConfig.Subnets,
				SecurityGroups: agentConfig.AWSFargateSpawnConfig.SecurityGroups,
				AssignPublicIp: assignPublicIP,
			},
		},
		Overrides: &ecstypes.TaskOverride{
			ContainerOverrides: []ecstypes.ContainerOverride{
				{
					Name: aws.String(agentConfig.AWSFargateSpawnConfig.ContainerName),
					Environment: []ecstypes.KeyValuePair{
						{Name: aws.String("AGENT_JOB_ID"), Value: aws.String(agentConfig.AgentJobID)},
						{Name: aws.String("AGENT_NAME"), Value: aws.String(agentConfig.AgentName)},
						{Name: aws.String("AGENT_PROMPT"), Value: aws.String(agentConfig.Prompt)},
						{Name: aws.String("AGENT_FARGATE_EVENTS_QUEUE_URL"), Value: aws.String(agentConfig.EventsQueueURL)},
					},
				},
			},
		},
		EnableECSManagedTags: true,
	}, nil
}

func validateAgentFargateJobConfig(agentConfig AgentFargateJobConfig) error {
	spawnConfig := agentConfig.AWSFargateSpawnConfig
	required := map[string]string{
		"Region":         spawnConfig.Region,
		"Cluster":        spawnConfig.Cluster,
		"TaskDefinition": spawnConfig.TaskDefinition,
		"ContainerName":  spawnConfig.ContainerName,
		"AgentJobID":     agentConfig.AgentJobID,
		"AgentName":      agentConfig.AgentName,
		"EventsQueueURL": agentConfig.EventsQueueURL,
	}

	var missing []string
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(spawnConfig.Subnets) == 0 {
		missing = append(missing, "Subnets")
	}
	if len(spawnConfig.SecurityGroups) == 0 {
		missing = append(missing, "SecurityGroups")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing Fargate job config fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

func formatECSFailures(failures []ecstypes.Failure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf("arn=%s reason=%s detail=%s", aws.ToString(failure.Arn), aws.ToString(failure.Reason), aws.ToString(failure.Detail)))
	}
	return strings.Join(parts, "; ")
}
