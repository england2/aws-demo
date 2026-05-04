# Local Tmux Watch

`local-tmux-watch` opens one local tmux window for each running conductor-spawned
Fargate agent task. Each window runs ECS Exec and attaches to the remote tmux
session named `agent-codex`.

## Prerequisites

- `python3`
- `tmux`
- AWS CLI v2
- AWS Session Manager plugin
- AWS credentials that can call `ecs:ListTasks`, `ecs:DescribeTasks`, and
  `ecs:ExecuteCommand`
- New conductor-spawned Fargate tasks with ECS Exec enabled

Existing tasks that were started before ECS Exec was enabled will be reported and
skipped.

## Usage

From this directory:

```bash
uv run python main.py
```

When run outside tmux, the watcher re-execs itself into a local tmux session named
`local-tmux-watch`. If that session already exists, tmux attaches to it. When run
from inside tmux, the watcher uses or creates the configured local session and
opens agent windows there.

The watcher polls ECS every 5 seconds. It only watches running tasks that were
started by `agent-conductor`, and it never closes local tmux windows. If a remote
task stops or ECS Exec disconnects, the local window stays open.

## Defaults

| Setting | Default | Flag | Environment |
| --- | --- | --- | --- |
| AWS region | `us-west-2` | `--region` | `LOCAL_TMUX_WATCH_REGION`, then `AWS_REGION`, then `AWS_DEFAULT_REGION` |
| ECS cluster | `ecs-cluster-agent-fargate` | `--cluster` | `LOCAL_TMUX_WATCH_CLUSTER` |
| ECS container | `agent-fargate` | `--container` | `LOCAL_TMUX_WATCH_CONTAINER` |
| Remote tmux session | `agent-codex` | `--remote-session` | `LOCAL_TMUX_WATCH_REMOTE_SESSION` |
| Local tmux session | `local-tmux-watch` | `--local-session` | `LOCAL_TMUX_WATCH_LOCAL_SESSION` |
| ECS `startedBy` filter | `agent-conductor` | `--started-by` | `LOCAL_TMUX_WATCH_STARTED_BY` |
| Poll interval | `5` seconds | `--poll` | `LOCAL_TMUX_WATCH_POLL_SECONDS` |

Useful one-shot validation:

```bash
uv run python main.py --once
```

## Window Command

Each local tmux window runs:

```bash
aws ecs execute-command --interactive \
  --region <region> \
  --cluster <cluster> \
  --task <task-arn> \
  --container <container> \
  --command "/bin/bash -lc 'tmux attach-session -t agent-codex || exec /bin/bash'"
```

The local window then drops into a shell after the remote command exits.

## Troubleshooting

- `does not have ECS Exec enabled`: start a new conductor Fargate agent after
  deploying the conductor change that sets `EnableExecuteCommand: true`.
- `does not contain container 'agent-fargate'`: pass the correct container name
  with `--container` or `LOCAL_TMUX_WATCH_CONTAINER`.
- ECS Exec fails immediately: confirm the task role, cluster, and local IAM
  identity allow ECS Exec and Systems Manager Session Manager access.
- `SessionManagerPlugin is not found`: install the AWS Session Manager plugin
  for AWS CLI v2.
- No windows appear: verify there are running tasks in
  `ecs-cluster-agent-fargate` with `startedBy=agent-conductor`.
