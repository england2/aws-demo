# ECS/Fargate notes for agent-conductor

## Goal

The conductor should spawn one isolated Fargate task per selected agent job.
Terraform should define stable infrastructure, while Go should perform the
dynamic operation:

```text
SQS message accepted
-> database creates agent job row
-> conductor builds per-job env vars
-> conductor calls ECS RunTask
-> Fargate runs agent-fargate container
-> container emits agent events back to conductor/SQS
```

## SDK packages

Use AWS SDK for Go v2:

```go
import (
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/ecs"
    "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)
```

The relevant client call is:

```go
ecsClient.RunTask(ctx, &ecs.RunTaskInput{...})
```

`RunTask` starts a new task from a task definition. For this project, that
means starting one `agent-fargate` worker container for one incident/job.

## Stable infrastructure Terraform should define

The conductor should not create all ECS resources from scratch for every job.
Terraform should define:

- ECS cluster, likely named `agent-fargate`.
- ECS task definition, likely family `agent-fargate`.
- Container name, likely `agent-fargate`.
- Task execution role for ECR pull and CloudWatch logs.
- Task role for runtime AWS access, including Secrets Manager read.
- Security group for the task.
- Subnets for task placement.
- CloudWatch log group.
- ECR repository/image.

The Go conductor should only call `RunTask` with dynamic job-specific values.

## Why RunTask, not service

Use `RunTask` for agent jobs because these are one-off workers, not a
long-running replicated service. Each alert/job maps naturally to one ECS task.

Services are better for "keep N copies of this server running." Agent jobs are
"run this isolated unit of work and exit."

## Minimum RunTask shape

```go
cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
if err != nil {
    return err
}

ecsClient := ecs.NewFromConfig(cfg)

out, err := ecsClient.RunTask(ctx, &ecs.RunTaskInput{
    Cluster:        aws.String("agent-fargate"),
    TaskDefinition: aws.String("agent-fargate"),
    LaunchType:     types.LaunchTypeFargate,
    Count:          aws.Int32(1),
    NetworkConfiguration: &types.NetworkConfiguration{
        AwsvpcConfiguration: &types.AwsVpcConfiguration{
            Subnets:        []string{"subnet-..."},
            SecurityGroups: []string{"sg-..."},
            AssignPublicIp: types.AssignPublicIpEnabled,
        },
    },
    Overrides: &types.TaskOverride{
        ContainerOverrides: []types.ContainerOverride{
            {
                Name: aws.String("agent-fargate"),
                Environment: []types.KeyValuePair{
                    {Name: aws.String("AGENT_JOB_ID"), Value: aws.String("job-123")},
                    {Name: aws.String("AGENT_NAME"), Value: aws.String("agent-fargate")},
                    {Name: aws.String("AGENT_PROMPT"), Value: aws.String("...")},
                    {Name: aws.String("AGENT_OPERATION_EVENTS_QUEUE_URL"), Value: aws.String("https://sqs.us-west-2.amazonaws.com/204772699175/agent-operation-events")},
                },
            },
        },
    },
    StartedBy: aws.String("agent-conductor"),
    EnableECSManagedTags: true,
})
if err != nil {
    return err
}

for _, failure := range out.Failures {
    // Treat RunTask failures as job start failures.
}

for _, task := range out.Tasks {
    // Store task.TaskArn on agent_job_info.
}
```

## Environment variables

Per-job environment variables should be passed through:

```go
types.TaskOverride{
    ContainerOverrides: []types.ContainerOverride{
        {
            Name: aws.String("agent-fargate"),
            Environment: []types.KeyValuePair{
                {Name: aws.String("AGENT_JOB_ID"), Value: aws.String(jobID)},
                {Name: aws.String("AGENT_NAME"), Value: aws.String(agentName)},
            },
        },
    },
}
```

Useful env vars for this project:

- `AGENT_JOB_ID`: database job ID.
- `AGENT_NAME`: logical agent/spawner name.
- `AGENT_PROMPT`: prompt or compact job instructions.
- `AGENT_OPERATION_EVENTS_QUEUE_URL`: where the worker reports lifecycle events.
- `SQS_MESSAGE_ID`: original SQS message row ID or external message ID.
- `EXTERNAL_EVENT_ID`: CloudWatch/EventBridge event ID if available.
- `ALARM_NAME`: CloudWatch alarm name when relevant.
- `TARGET_REPOSITORY`: repo the agent should inspect.
- `TARGET_BRANCH`: branch base, usually `main`.
- `OUTPUT_MODE`: e.g. `pull_request`, `report_only`, `rollback`.

Avoid passing secrets as plain environment override values. For OpenAI auth,
the current plan is better:

```text
Fargate task role can call Secrets Manager
-> Go wrapper calls secretsmanager:GetSecretValue
-> Go wrapper injects OPENAI_API_KEY only into the child codex process env
```

## Network configuration

Fargate tasks using `awsvpc` networking need network config at run time. The SDK
type is:

```go
types.NetworkConfiguration{
    AwsvpcConfiguration: &types.AwsVpcConfiguration{
        Subnets:        []string{"subnet-..."},
        SecurityGroups: []string{"sg-..."},
        AssignPublicIp: types.AssignPublicIpEnabled,
    },
}
```

Notes:

- Subnets and security groups must be from the same VPC.
- `AwsVpcConfiguration.Subnets` is required.
- Public IP can be enabled for simple outbound internet access in public
  subnets.
- A better production-ish setup is private subnets plus NAT or VPC endpoints.

## Idempotency

`RunTaskInput.ClientToken` is the idempotency token. Use a deterministic token
derived from the database job ID:

```go
ClientToken: aws.String("agent-job-" + jobID)
```

This prevents accidental duplicate task starts if the conductor retries after a
transient failure. If the same client token is reused with different parameters,
ECS can return a conflict.

## Tracking spawned tasks

After `RunTask`, store the ECS task ARN:

```text
agent_job_info.ecs_task_arn
agent_job_info.status = "running"
sqs_message.job_status = "running"
```

Then use:

- `DescribeTasks` to check status.
- `StopTask` to kill a job.
- `ListTasks` for debugging or reconciliation.

Important statuses to care about:

- `PROVISIONING`
- `PENDING`
- `RUNNING`
- `STOPPED`

If `RunTaskOutput.Failures` is non-empty, the job should be marked failed even
if the API call itself did not return an error.

## Eventual consistency

ECS is eventually consistent. After `RunTask`, `DescribeTasks` may not
immediately show the final desired state. Use retry/backoff when polling task
state. Do not assume immediate visibility after task creation.

## IAM needed by the conductor

The EC2 conductor role will need permission to:

- `ecs:RunTask`
- `ecs:DescribeTasks`
- `ecs:StopTask`
- likely `ecs:ListTasks`
- `iam:PassRole` for the task role and task execution role

`iam:PassRole` is easy to forget. ECS needs it because the conductor is asking
ECS to run a task using specific IAM roles.

## IAM needed by the Fargate task

The Fargate task role needs runtime permissions for the worker process:

- `secretsmanager:GetSecretValue` for `openai-key-aws-demo-agent-fargate-*`.
- SQS send permission if the worker emits events directly to SQS.
- GitHub credentials are not AWS IAM; those need separate handling.

The task execution role needs platform permissions:

- Pull image from ECR.
- Write logs to CloudWatch.
- Read ECS-injected secrets if using ECS secret injection.

Because the wrapper fetches Secrets Manager itself, the OpenAI secret permission
belongs on the task role.

## Recommended conductor function shape

```go
type FargateJobConfig struct {
    Cluster              string
    TaskDefinition       string
    ContainerName        string
    Subnets              []string
    SecurityGroups       []string
    AssignPublicIP       bool
    JobID                string
    AgentName            string
    Prompt               string
    EventsQueueURL       string
}

func SpawnFargateAgent(ctx context.Context, cfg FargateJobConfig) (string, error) {
    // load AWS config
    // build []types.KeyValuePair from cfg
    // call ecs.RunTask
    // check out.Failures
    // return task ARN
}
```

Return the task ARN and persist it in SQLite.

## Source notes

- ECS Go SDK package docs: `github.com/aws/aws-sdk-go-v2/service/ecs`.
- `RunTask` starts a new task using a task definition.
- `RegisterTaskDefinition` supports task roles and `awsvpc` networking.
- `RunTaskInput.TaskDefinition` is required.
- `RunTaskInput.Count` can start up to 10 task instances per call.
- `types.ContainerOverride.Environment` passes env vars with
  `[]types.KeyValuePair`.
- `types.AwsVpcConfiguration` carries subnets, security groups, and public IP
  assignment.
