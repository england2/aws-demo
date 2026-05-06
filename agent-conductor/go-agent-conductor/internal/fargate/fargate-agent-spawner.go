package fargate

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// AWSFargateSpawnConfig is the static ECS infrastructure config needed to run a task.
// It is currently populated from adhoc-config.go and mirrors Terraform-created resources.
type AWSFargateSpawnConfig struct {
	Region         string
	Cluster        string
	TaskDefinition string
	ContainerName  string
	Subnets        []string
	SecurityGroups []string
	AssignPublicIP bool
}

// AgentFargateJobConfig combines static ECS config with per-agent runtime config.
// main builds this immediately before spawning one Fargate task for one agent job.
type AgentFargateJobConfig struct {
	AWSFargateSpawnConfig AWSFargateSpawnConfig

	RuntimeEnv AgentFargateRuntimeEnv
}

// AgentFargateRuntimeEnv is the dynamic configuration injected into the Fargate container.
// The wrapper reads these env vars to identify its job, prompt Codex, and emit events.
type AgentFargateRuntimeEnv struct {
	AgentJobID              string
	AgentName               string
	Prompt                  string
	EventsQueueURL          string
	DebugSSHEnabled         bool
	DebugSSHPublicKeySecret string
}

// AgentFargateSpawnResult is the minimal ECS result the conductor needs after RunTask.
// The task ARN is persisted and used by the running-agent monitor.
type AgentFargateSpawnResult struct {
	TaskARN string
}

// SpawnFargateAgent starts one ECS Fargate task for an approved agent job.
// It depends on AWS SDK credentials from the conductor EC2 role and returns only
// after ECS accepts the task and supplies a task ARN.
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

// BuildRunTaskInput turns conductor config into an ECS RunTask request.
// It is separated from SpawnFargateAgent so tests can validate AWS request shape
// without calling ECS.
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
		StartedBy:      aws.String("agent-conductor"),
		// Required so local operators can attach with ECS Exec to the remote tmux session.
		EnableExecuteCommand: true,
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
					Name:        aws.String(agentConfig.AWSFargateSpawnConfig.ContainerName),
					Environment: agentConfig.RuntimeEnv.ECSEnvironment(),
				},
			},
		},
		EnableECSManagedTags: true,
	}, nil
}

// validateAgentFargateJobConfig fails fast on missing ECS or runtime job fields.
// This prevents launching partially configured Fargate tasks that cannot report back.
func validateAgentFargateJobConfig(agentConfig AgentFargateJobConfig) error {
	spawnConfig := agentConfig.AWSFargateSpawnConfig
	runtimeEnv := agentConfig.RuntimeEnv
	required := map[string]string{
		"Region":         spawnConfig.Region,
		"Cluster":        spawnConfig.Cluster,
		"TaskDefinition": spawnConfig.TaskDefinition,
		"ContainerName":  spawnConfig.ContainerName,
		"AgentJobID":     runtimeEnv.AgentJobID,
		"AgentName":      runtimeEnv.AgentName,
		"EventsQueueURL": runtimeEnv.EventsQueueURL,
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

// ECSEnvironment converts runtime config into ECS container environment overrides.
// These values are the Fargate worker's control plane: job identity, prompt, event queue,
// and optional debug SSH behavior.
func (runtimeEnv AgentFargateRuntimeEnv) ECSEnvironment() []ecstypes.KeyValuePair {
	environment := []ecstypes.KeyValuePair{
		{Name: aws.String("AGENT_JOB_ID"), Value: aws.String(runtimeEnv.AgentJobID)},
		{Name: aws.String("AGENT_NAME"), Value: aws.String(runtimeEnv.AgentName)},
		{Name: aws.String("AGENT_PROMPT"), Value: aws.String(runtimeEnv.Prompt)},
		{Name: aws.String("AGENT_FARGATE_EVENTS_QUEUE_URL"), Value: aws.String(runtimeEnv.EventsQueueURL)},
	}
	if runtimeEnv.DebugSSHEnabled {
		secretName := strings.TrimSpace(runtimeEnv.DebugSSHPublicKeySecret)
		if secretName == "" {
			secretName = "debug_public_ssh_key"
		}
		environment = append(
			environment,
			ecstypes.KeyValuePair{Name: aws.String("DEBUG_SSH_ENABLED"), Value: aws.String("true")},
			ecstypes.KeyValuePair{Name: aws.String("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"), Value: aws.String(secretName)},
		)
	}
	return environment
}

// DebugSSHRuntimeEnv reads optional conductor env vars that enable debug SSH in workers.
// The values are passed through to the Fargate wrapper only when explicitly enabled.
func DebugSSHRuntimeEnv() (bool, string) {
	return truthyEnv("DEBUG_SSH_ENABLED"), getenvDefault("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", "debug_public_ssh_key")
}

// getenvDefault reads an environment variable with a fallback for Fargate runtime config.
func getenvDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

// truthyEnv normalizes shell-friendly boolean env values.
// It is used for debug feature flags, not security-critical authorization.
func truthyEnv(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// formatECSFailures renders ECS RunTask failures into one operator-readable string.
// SpawnFargateAgent includes this in returned errors for SSM/systemd logs.
func formatECSFailures(failures []ecstypes.Failure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf("arn=%s reason=%s detail=%s", aws.ToString(failure.Arn), aws.ToString(failure.Reason), aws.ToString(failure.Detail)))
	}
	return strings.Join(parts, "; ")
}
