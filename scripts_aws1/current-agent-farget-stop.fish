#!/usr/bin/env fish

set state_path ~/programming/aws1/scripts/ignore.current-test-fargate.txt
if set -q AGENT_FARGATE_STATE_PATH
    set state_path $AGENT_FARGATE_STATE_PATH
end

if not test -f $state_path
    echo "missing Fargate state file: $state_path" >&2
    exit 1
end

set region us-west-2
set cluster ""
set task_arn ""
set task_definition_arn ""

for line in (cat $state_path)
    if test -z "$line"
        continue
    end

    set parts (string split -m 1 "=" -- $line)
    if test (count $parts) -ne 2
        continue
    end

    switch $parts[1]
        case region
            set region $parts[2]
        case cluster
            set cluster $parts[2]
        case task_arn
            set task_arn $parts[2]
        case task_definition_arn
            set task_definition_arn $parts[2]
    end
end

if test -z "$cluster"
    echo "$state_path is missing cluster" >&2
    exit 1
end

if test -z "$task_arn"
    echo "$state_path is missing task_arn" >&2
    exit 1
end

echo "stopping Fargate task: $task_arn"
aws ecs stop-task \
    --region $region \
    --cluster $cluster \
    --task $task_arn \
    --reason "stopped aggressively by scripts/stop-fargate-aws.fish" \
    >/dev/null

for attempt in (seq 1 30)
    set task_status (aws ecs describe-tasks \
        --region $region \
        --cluster $cluster \
        --tasks $task_arn \
        --query 'tasks[0].lastStatus' \
        --output text 2>/dev/null)

    if test "$task_status" = "STOPPED" -o "$task_status" = "None" -o -z "$task_status"
        break
    end

    echo "waiting for STOPPED: $task_status"
    sleep 2
end

if test -n "$task_definition_arn"
    echo "deregistering temporary task definition: $task_definition_arn"
    aws ecs deregister-task-definition \
        --region $region \
        --task-definition $task_definition_arn \
        >/dev/null
end

rm -f $state_path
echo "stopped and removed $state_path"
