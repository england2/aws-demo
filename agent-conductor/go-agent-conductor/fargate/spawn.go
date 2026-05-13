package fargate

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// SpawnConfig is the static ECS infrastructure config needed to run one task.
type SpawnConfig struct {
	Region         string
	Cluster        string
	TaskDefinition string
	ContainerName  string
	Subnets        []string
	SecurityGroups []string
	AssignPublicIP bool
}

// SpawnRequest is all caller-provided state for one Fargate task spawn.
type SpawnRequest struct {
	Config      SpawnConfig
	Environment map[string]string
}

// SpawnResult is the ECS task identity returned after a successful RunTask call.
type SpawnResult struct {
	TaskARN string
}

type ecsClient interface {
	// RunTask is the only ECS operation this package needs for the no-state spawn path.
	// Spawn calls it after BuildRunTaskInput and AWS client construction have completed, and the returned task
	// list becomes the sole source for SpawnResult.TaskARN.
	RunTask(ctx context.Context, params *ecs.RunTaskInput, optFns ...func(*ecs.Options)) (*ecs.RunTaskOutput, error)
}

// loadAWSConfig is the replaceable AWS config loader used immediately before ECS client creation.
// Production code uses the SDK default chain for the request region, while tests install a deterministic hook
// before calling Spawn so no external AWS lookup occurs.
var loadAWSConfig = func(ctx context.Context, region string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}

// newECSClient is the replaceable constructor that converts loaded AWS config into the RunTask client.
// Spawn calls it after loadAWSConfig succeeds, and tests replace it so RunTask input and output stay in memory.
var newECSClient = func(awsConfig aws.Config) ecsClient {
	return ecs.NewFromConfig(awsConfig)
}

// Spawn is the package boundary that turns a validated SpawnRequest into one ECS RunTask call.
// Callers must establish config and environment first; Spawn builds the ECS input, loads AWS config for the
// request region, and returns only the task ARN needed by the conductor log/delete path.
func Spawn(ctx context.Context, request SpawnRequest) (SpawnResult, error) {
	input, err := BuildRunTaskInput(request)
	if err != nil {
		return SpawnResult{}, err
	}

	awsConfig, err := loadAWSConfig(ctx, request.Config.Region)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("load AWS config for Fargate spawn: %w", err)
	}

	output, err := newECSClient(awsConfig).RunTask(ctx, input)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("run Fargate task: %w", err)
	}
	if len(output.Failures) > 0 {
		return SpawnResult{}, fmt.Errorf("run Fargate task failed: %s", formatECSFailures(output.Failures))
	}
	if len(output.Tasks) != 1 {
		return SpawnResult{}, fmt.Errorf("expected one Fargate task, got %d", len(output.Tasks))
	}

	taskARN := aws.ToString(output.Tasks[0].TaskArn)
	if taskARN == "" {
		return SpawnResult{}, fmt.Errorf("ECS returned task without task ARN")
	}

	return SpawnResult{TaskARN: taskARN}, nil
}

// BuildRunTaskInput converts static ECS config plus container env into the exact RunTask payload.
// Spawn calls it before AWS credentials are loaded, and tests call it directly to lock request shape without
// reaching ECS.
func BuildRunTaskInput(request SpawnRequest) (*ecs.RunTaskInput, error) {
	if err := validateSpawnRequest(request); err != nil {
		return nil, err
	}

	assignPublicIP := ecstypes.AssignPublicIpDisabled
	if request.Config.AssignPublicIP {
		assignPublicIP = ecstypes.AssignPublicIpEnabled
	}

	environment := make([]ecstypes.KeyValuePair, 0, len(request.Environment))
	keys := make([]string, 0, len(request.Environment))
	for key := range request.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, ecstypes.KeyValuePair{
			Name:  aws.String(key),
			Value: aws.String(request.Environment[key]),
		})
	}

	return &ecs.RunTaskInput{
		Cluster:        aws.String(request.Config.Cluster),
		TaskDefinition: aws.String(request.Config.TaskDefinition),
		LaunchType:     ecstypes.LaunchTypeFargate,
		Count:          aws.Int32(1),
		StartedBy:      aws.String("agent-conductor"),
		// Required so local operators can attach with ECS Exec to the remote tmux session.
		EnableExecuteCommand: true,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        request.Config.Subnets,
				SecurityGroups: request.Config.SecurityGroups,
				AssignPublicIp: assignPublicIP,
			},
		},
		Overrides: &ecstypes.TaskOverride{
			ContainerOverrides: []ecstypes.ContainerOverride{{
				Name:        aws.String(request.Config.ContainerName),
				Environment: environment,
			}},
		},
		EnableECSManagedTags: true,
	}, nil
}

// WithDebugSSHEnvironment copies the caller's container env and appends debug SSH settings when enabled.
// HandleSQSMessage and the smoke entrypoint call it after AGENT_NAME and AGENT_PROMPT are established, so the
// final map can flow unchanged into BuildRunTaskInput.
func WithDebugSSHEnvironment(environment map[string]string) map[string]string {
	next := make(map[string]string, len(environment)+2)
	for key, value := range environment {
		next[key] = value
	}

	if !truthyEnv("DEBUG_SSH_ENABLED") {
		return next
	}

	secretName := strings.TrimSpace(os.Getenv("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"))
	if secretName == "" {
		secretName = "debug_public_ssh_key"
	}
	next["DEBUG_SSH_ENABLED"] = "true"
	next["DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"] = secretName
	return next
}

// validateSpawnRequest fails before ECS input construction when required infrastructure or runtime env is absent.
// BuildRunTaskInput calls it first so missing queue-derived prompt data or adhoc cluster config cannot reach
// fargate.Spawn's AWS RunTask call.
func validateSpawnRequest(request SpawnRequest) error {
	required := map[string]string{
		"Region":         request.Config.Region,
		"Cluster":        request.Config.Cluster,
		"TaskDefinition": request.Config.TaskDefinition,
		"ContainerName":  request.Config.ContainerName,
		"AGENT_NAME":     request.Environment["AGENT_NAME"],
		"AGENT_PROMPT":   request.Environment["AGENT_PROMPT"],
	}

	var missing []string
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(request.Config.Subnets) == 0 {
		missing = append(missing, "Subnets")
	}
	if len(request.Config.SecurityGroups) == 0 {
		missing = append(missing, "SecurityGroups")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing Fargate spawn fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

// truthyEnv centralizes shell-friendly parsing for feature flags used during spawn request assembly.
// WithDebugSSHEnvironment calls it before adding debug env vars, keeping normal worker spawns free of optional
// SSH settings unless the conductor process opted in.
func truthyEnv(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// formatECSFailures compresses ECS RunTask failure records into one operator-readable error string.
// Spawn calls it after RunTask returns API-level failures, so main and smoke callers receive actionable context
// without needing to inspect AWS SDK structs.
func formatECSFailures(failures []ecstypes.Failure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf("arn=%s reason=%s detail=%s", aws.ToString(failure.Arn), aws.ToString(failure.Reason), aws.ToString(failure.Detail)))
	}
	return strings.Join(parts, "; ")
}
