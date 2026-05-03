#!/usr/bin/env fish

# Docker's default bridge network allows outbound HTTPS/DNS. Keep it explicit so
# local Codex/OpenAI, AWS Secrets Manager, GitHub, and package-manager requests
# are not blocked by an accidental --network none style override.
docker run -it \
        --network bridge \
        --entrypoint /bin/bash \
        -e AWS_REGION=us-west-2 \
        -e AWS_DEFAULT_REGION=us-west-2 \
        -e AWS_PROFILE=default \
        -e AWS_SDK_LOAD_CONFIG=true \
        -e AWS_EC2_METADATA_DISABLED=true \
        -e OPENAI_API_KEY \
        -v ~/.aws:/root/.aws:ro \
        agent-fargate:latest
