#!/usr/bin/env fish

set script_dir (cd (dirname (status --current-filename)); pwd)
set repo_root (cd "$script_dir/.."; pwd)
set dockerfile "$repo_root/agent-conductor/agent-fargate/container/Dockerfile"

set image_name agent-fargate:latest
if test (count $argv) -ge 1
    set image_name $argv[1]
end

if not test -f "$dockerfile"
    echo "missing Dockerfile: $dockerfile" >&2
    exit 1
end

if not test -d "$repo_root/agent-conductor/shared/agentproto"
    echo "missing shared agentproto package; build context must be repo root: $repo_root" >&2
    exit 1
end

docker build \
    --platform linux/amd64 \
    -f "$dockerfile" \
    "$repo_root" \
    -t "$image_name"
