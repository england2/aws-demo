package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// AWSFargateSpawnConfig is the static ECS infrastructure config needed to run a task.
// It is currently populated from adhoc-config.go and mirrors Terraform-created resources.
//
// ai--done: active worker lifecycle ownership moved to FargateAgentWorker,
// while this remains a readable nested config field.
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
// spawnAndTrackAgentJob builds this explicitly before creating the FargateAgentWorker.
type AgentFargateJobConfig struct {
	AWSFargateSpawnConfig AWSFargateSpawnConfig

	RuntimeEnv AgentFargateRuntimeEnv
}

// AgentFargateRuntimeEnv is the dynamic configuration injected into the Fargate container.
// The wrapper reads these env vars to identify its job and prompt Codex.
type AgentFargateRuntimeEnv struct {
	AgentJobID              string
	AgentName               string
	Prompt                  string
	DebugSSHEnabled         bool
	DebugSSHPublicKeySecret string
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

// DebugSSHRuntimeEnv reads optional conductor env vars that enable debug SSH in workers.
// The values are passed through to the Fargate wrapper only when explicitly enabled.
func DebugSSHRuntimeEnv() (bool, string) {
	return truthyEnv("DEBUG_SSH_ENABLED"), getenvDefault("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", "debug_public_ssh_key")
}

// truthyEnv normalizes shell-friendly boolean env values.
// It is used for debug feature flags, not security-critical authorization.
func truthyEnv(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// formatECSFailures renders ECS RunTask failures into one operator-readable string.
// spawnAndTrackAgentJob includes this in returned errors for SSM/systemd logs.
func formatECSFailures(failures []ecstypes.Failure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf("arn=%s reason=%s detail=%s", aws.ToString(failure.Arn), aws.ToString(failure.Reason), aws.ToString(failure.Detail)))
	}
	return strings.Join(parts, "; ")
}
