#!/usr/bin/env fish

set script_dir (cd (dirname (status --current-filename)); pwd)
set repo_root (cd "$script_dir/.."; pwd)

set image_name agent-fargate:latest
if test (count $argv) -ge 1
    set image_name $argv[1]
end

docker build \
    --platform linux/amd64 \
    -f "$repo_root/agent-conductor/agent-fargate/container/Dockerfile" \
    "$repo_root/agent-conductor" \
    -t "$image_name"
