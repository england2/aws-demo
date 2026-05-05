package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ec2svc "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	defaultRegion            = "us-west-2"
	defaultCluster           = "ecs-cluster-agent-fargate"
	defaultRepository        = "agent-fargate"
	defaultImageTag          = "latest"
	defaultContainerName     = "agent-fargate"
	defaultTaskFamily        = "agent-fargate-exec-test"
	defaultSecurityGroupName = "agent-fargate-sg"
	defaultTaskRoleName      = "agent-fargate-task-role"
	defaultExecutionRoleName = "agent-fargate-execution-role"
	defaultAgentJobID        = "manual-test"
	defaultAgentName         = "agent-fargate-codex"
	defaultAgentPrompt       = "manual Fargate test task"
	defaultAgentEventsQueue  = "https://sqs.us-west-2.amazonaws.com/204772699175/agent-fargate-events"
	defaultDebugSSHSecret    = "debug_public_ssh_key"
)

func main() {
	ctx := context.Background()
	settings, err := loadSettings()
	if err != nil {
		fatal(err)
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(settings.Region))
	if err != nil {
		fatal(fmt.Errorf("load AWS config: %w", err))
	}

	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		fatal(fmt.Errorf("get AWS caller identity: %w", err))
	}

	accountID := aws.ToString(identity.Account)
	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s", accountID, settings.Region, settings.Repository, settings.ImageTag)
	taskRoleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, settings.TaskRoleName)
	executionRoleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, settings.ExecutionRoleName)

	if err := requireECRImage(ctx, ecr.NewFromConfig(cfg), settings.Repository, settings.ImageTag); err != nil {
		fatal(err)
	}

	ec2Client := ec2svc.NewFromConfig(cfg)
	vpcID, err := defaultVPCID(ctx, ec2Client)
	if err != nil {
		fatal(err)
	}

	subnetIDs, err := defaultSubnetIDs(ctx, ec2Client, vpcID)
	if err != nil {
		fatal(err)
	}

	securityGroupID, err := securityGroupIDByName(ctx, ec2Client, vpcID, settings.SecurityGroupName)
	if err != nil {
		fatal(err)
	}

	ecsClient := ecs.NewFromConfig(cfg)
	taskDefinitionArn, err := registerExecTestTaskDefinition(ctx, ecsClient, taskDefinitionConfig{
		Family:           settings.TaskFamily,
		ContainerName:    settings.ContainerName,
		ImageURI:         imageURI,
		TaskRoleArn:      taskRoleArn,
		ExecutionRoleArn: executionRoleArn,
		RuntimeEnv:       settings.RuntimeEnv(),
	})
	if err != nil {
		fatal(err)
	}

	runOut, err := ecsClient.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:              aws.String(settings.Cluster),
		TaskDefinition:       aws.String(taskDefinitionArn),
		LaunchType:           ecstypes.LaunchTypeFargate,
		Count:                aws.Int32(1),
		EnableExecuteCommand: true,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        subnetIDs,
				SecurityGroups: []string{securityGroupID},
				AssignPublicIp: ecstypes.AssignPublicIpEnabled,
			},
		},
		StartedBy:            aws.String("start-fargate-aws"),
		EnableECSManagedTags: true,
	})
	if err != nil {
		fatal(fmt.Errorf("run ECS task: %w", err))
	}
	if len(runOut.Failures) > 0 {
		fatal(fmt.Errorf("run ECS task failed: %s", formatFailures(runOut.Failures)))
	}
	if len(runOut.Tasks) != 1 {
		fatal(fmt.Errorf("expected one started task, got %d", len(runOut.Tasks)))
	}

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)
	if err := waitForTaskRunning(ctx, ecsClient, settings.Cluster, taskArn); err != nil {
		fatal(err)
	}

	connectCommand := fmt.Sprintf("scripts/current-agent-farget-connect.fish")

	if err := writeState(settings.StatePath, map[string]string{
		"region":              settings.Region,
		"cluster":             settings.Cluster,
		"task_arn":            taskArn,
		"task_definition_arn": taskDefinitionArn,
		"container_name":      settings.ContainerName,
		"security_group_id":   securityGroupID,
		"subnet_ids":          strings.Join(subnetIDs, ","),
		"connect_command":     connectCommand,
		"started_at":          time.Now().Format(time.RFC3339),
	}); err != nil {
		fatal(err)
	}

	fmt.Printf("started task: %s\n", taskArn)
	fmt.Printf("wrote %s\n", settings.StatePath)
	fmt.Printf("connect with: %s\n", connectCommand)
	fmt.Println("then run inside the container: /entrypoint.sh")
}

type settings struct {
	Region            string
	Cluster           string
	Repository        string
	ImageTag          string
	ContainerName     string
	TaskFamily        string
	SecurityGroupName string
	TaskRoleName      string
	ExecutionRoleName string
	AgentJobID        string
	AgentName         string
	AgentPrompt       string
	AgentEventsQueue  string
	DebugSSHEnabled   bool
	DebugSSHSecret    string
	StatePath         string
}

type taskDefinitionConfig struct {
	Family           string
	ContainerName    string
	ImageURI         string
	TaskRoleArn      string
	ExecutionRoleArn string
	RuntimeEnv       []ecstypes.KeyValuePair
}

func loadSettings() (settings, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return settings{}, fmt.Errorf("resolve home dir: %w", err)
	}

	return settings{
		Region:            envDefault("AWS_REGION", defaultRegion),
		Cluster:           envDefault("AGENT_FARGATE_CLUSTER", defaultCluster),
		Repository:        envDefault("AGENT_FARGATE_REPOSITORY", defaultRepository),
		ImageTag:          envDefault("AGENT_FARGATE_IMAGE_TAG", defaultImageTag),
		ContainerName:     envDefault("AGENT_FARGATE_CONTAINER_NAME", defaultContainerName),
		TaskFamily:        envDefault("AGENT_FARGATE_EXEC_TASK_FAMILY", defaultTaskFamily),
		SecurityGroupName: envDefault("AGENT_FARGATE_SECURITY_GROUP_NAME", defaultSecurityGroupName),
		TaskRoleName:      envDefault("AGENT_FARGATE_TASK_ROLE_NAME", defaultTaskRoleName),
		ExecutionRoleName: envDefault("AGENT_FARGATE_EXECUTION_ROLE_NAME", defaultExecutionRoleName),
		AgentJobID:        envDefault("AGENT_JOB_ID", defaultAgentJobID),
		AgentName:         envDefault("AGENT_NAME", defaultAgentName),
		AgentPrompt:       envDefault("AGENT_PROMPT", defaultAgentPrompt),
		AgentEventsQueue:  envDefault("AGENT_FARGATE_EVENTS_QUEUE_URL", defaultAgentEventsQueue),
		DebugSSHEnabled:   truthyEnv("DEBUG_SSH_ENABLED"),
		DebugSSHSecret:    envDefault("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME", defaultDebugSSHSecret),
		StatePath:         expandHome(envDefault("AGENT_FARGATE_STATE_PATH", filepath.Join(home, "programming", "aws1", "scripts", "ignore.current-test-fargate.txt")), home),
	}, nil
}

func (s settings) RuntimeEnv() []ecstypes.KeyValuePair {
	environment := []ecstypes.KeyValuePair{
		{Name: aws.String("AWS_REGION"), Value: aws.String(s.Region)},
		{Name: aws.String("AGENT_JOB_ID"), Value: aws.String(s.AgentJobID)},
		{Name: aws.String("AGENT_NAME"), Value: aws.String(s.AgentName)},
		{Name: aws.String("AGENT_PROMPT"), Value: aws.String(s.AgentPrompt)},
		{Name: aws.String("AGENT_FARGATE_EVENTS_QUEUE_URL"), Value: aws.String(s.AgentEventsQueue)},
	}
	if s.DebugSSHEnabled {
		environment = append(
			environment,
			ecstypes.KeyValuePair{Name: aws.String("DEBUG_SSH_ENABLED"), Value: aws.String("true")},
			ecstypes.KeyValuePair{Name: aws.String("DEBUG_SSH_PUBLIC_KEY_SECRET_NAME"), Value: aws.String(s.DebugSSHSecret)},
		)
	}
	return environment
}

func registerExecTestTaskDefinition(ctx context.Context, client *ecs.Client, cfg taskDefinitionConfig) (string, error) {
	out, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(cfg.Family),
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		Cpu:                     aws.String("1024"),
		Memory:                  aws.String("2048"),
		TaskRoleArn:             aws.String(cfg.TaskRoleArn),
		ExecutionRoleArn:        aws.String(cfg.ExecutionRoleArn),
		RuntimePlatform: &ecstypes.RuntimePlatform{
			CpuArchitecture:       ecstypes.CPUArchitectureArm64,
			OperatingSystemFamily: ecstypes.OSFamilyLinux,
		},
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:        aws.String(cfg.ContainerName),
				Image:       aws.String(cfg.ImageURI),
				Essential:   aws.Bool(true),
				EntryPoint:  []string{"/bin/bash", "-lc"},
				Command:     []string{"trap : TERM INT; sleep infinity & wait"},
				Environment: cfg.RuntimeEnv,
				PortMappings: []ecstypes.PortMapping{
					{
						ContainerPort: aws.Int32(22),
						HostPort:      aws.Int32(22),
						Protocol:      ecstypes.TransportProtocolTcp,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("register temporary ECS Exec task definition: %w", err)
	}

	return aws.ToString(out.TaskDefinition.TaskDefinitionArn), nil
}

func requireECRImage(ctx context.Context, client *ecr.Client, repository string, tag string) error {
	_, err := client.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repository),
		ImageIds: []ecrtypes.ImageIdentifier{
			{
				ImageTag: aws.String(tag),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("ECR image %s:%s is not available; push the agent-fargate image before starting a task: %w", repository, tag, err)
	}
	return nil
}

func defaultVPCID(ctx context.Context, client *ec2svc.Client) (string, error) {
	out, err := client.DescribeVpcs(ctx, &ec2svc.DescribeVpcsInput{
		Filters: []ec2types.Filter{{Name: aws.String("is-default"), Values: []string{"true"}}},
	})
	if err != nil {
		return "", fmt.Errorf("describe default VPC: %w", err)
	}
	if len(out.Vpcs) != 1 {
		return "", fmt.Errorf("expected one default VPC, got %d", len(out.Vpcs))
	}
	return aws.ToString(out.Vpcs[0].VpcId), nil
}

func defaultSubnetIDs(ctx context.Context, client *ec2svc.Client, vpcID string) ([]string, error) {
	out, err := client.DescribeSubnets(ctx, &ec2svc.DescribeSubnetsInput{
		Filters: []ec2types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})
	if err != nil {
		return nil, fmt.Errorf("describe default VPC subnets: %w", err)
	}
	if len(out.Subnets) == 0 {
		return nil, fmt.Errorf("default VPC %s has no subnets", vpcID)
	}

	subnetIDs := make([]string, 0, len(out.Subnets))
	for _, subnet := range out.Subnets {
		subnetIDs = append(subnetIDs, aws.ToString(subnet.SubnetId))
	}
	return subnetIDs, nil
}

func securityGroupIDByName(ctx context.Context, client *ec2svc.Client, vpcID string, name string) (string, error) {
	out, err := client.DescribeSecurityGroups(ctx, &ec2svc.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("group-name"), Values: []string{name}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe security group %s: %w", name, err)
	}
	if len(out.SecurityGroups) != 1 {
		return "", fmt.Errorf("expected one security group named %s in VPC %s, got %d; run terraform apply after adding agent-fargate-sg", name, vpcID, len(out.SecurityGroups))
	}
	return aws.ToString(out.SecurityGroups[0].GroupId), nil
}

func waitForTaskRunning(ctx context.Context, client *ecs.Client, cluster string, taskArn string) error {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   []string{taskArn},
		})
		if err != nil {
			return fmt.Errorf("describe task: %w", err)
		}
		if len(out.Tasks) != 1 {
			return fmt.Errorf("expected one described task, got %d", len(out.Tasks))
		}

		task := out.Tasks[0]
		status := aws.ToString(task.LastStatus)
		if status == "RUNNING" {
			return nil
		}
		if status == "STOPPED" {
			return fmt.Errorf("task stopped before becoming executable: %s", aws.ToString(task.StoppedReason))
		}

		fmt.Printf("waiting for task RUNNING: status=%s\n", status)
		time.Sleep(10 * time.Second)
	}
	return errors.New("timed out waiting for task to reach RUNNING")
}

func writeState(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create state file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()
	for _, key := range []string{"region", "cluster", "task_arn", "task_definition_arn", "container_name", "security_group_id", "subnet_ids", "connect_command", "started_at"} {
		if _, err := fmt.Fprintf(writer, "%s=%s\n", key, values[key]); err != nil {
			return fmt.Errorf("write state file: %w", err)
		}
	}
	return nil
}

func formatFailures(failures []ecstypes.Failure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf("arn=%s reason=%s detail=%s", aws.ToString(failure.Arn), aws.ToString(failure.Reason), aws.ToString(failure.Detail)))
	}
	return strings.Join(parts, "; ")
}

func envDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func truthyEnv(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func expandHome(path string, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
