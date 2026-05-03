#!/usr/bin/env fish
docker run -it \
        --entrypoint /bin/bash \
        -e AWS_REGION=us-west-2 \
        -e AWS_PROFILE=default \
        -e AWS_EC2_METADATA_DISABLED=true \
        -v ~/.aws:/root/.aws:ro \
        agent-fargate:latest
