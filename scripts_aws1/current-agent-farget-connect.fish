#!/usr/bin/env fish


# depends on AWS Session Manager plugin:
# fedora install:
#   curl -Lo /tmp/session-manager-plugin.rpm https://s3.amazonaws.com/session-manager-downloads/plugin/latest/linux_64bit/session-manager-plugin.rpm
#   sudo dnf install -y /tmp/session-manager-plugin.rpm

set state_path ~/programming/aws1/scripts/ignore.current-test-fargate.txt
if set -q AGENT_FARGATE_STATE_PATH
    set state_path $AGENT_FARGATE_STATE_PATH
end

if not test -f $state_path
    echo "missing Fargate state file: $state_path" >&2
    echo "run scripts/start-fargate-aws first" >&2
    exit 1
end

set region us-west-2
set cluster ""
set task_arn ""
set container_name agent-fargate

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
        case container_name
            set container_name $parts[2]
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

aws ecs execute-command \
    --region $region \
    --cluster $cluster \
    --task $task_arn \
    --container $container_name \
    --interactive \
    --command "/bin/bash"
