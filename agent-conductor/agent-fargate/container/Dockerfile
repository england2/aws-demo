# syntax=docker/dockerfile:1

# build stage
# =============================================================

ARG TARGETARCH

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETARCH

WORKDIR /src

COPY agent-conductor/agent-fargate/container/go-codex-wrapper/go.mod agent-conductor/agent-fargate/container/go-codex-wrapper/go.sum ./
COPY agent-conductor/shared/agentproto/ /shared/agentproto/
COPY agent-conductor/agent-fargate/container/go-codex-wrapper/*.go ./
COPY agent-conductor/agent-fargate/container/go-codex-wrapper/tools/ ./tools/
COPY agent-conductor/agent-fargate/container/go-codex-wrapper/cmd/ ./cmd/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o /out/codex-wrapper .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o /out/get-secrets ./cmd/get-secrets

# runtime container
# =============================================================

FROM debian:bookworm-slim

ENV PATH="/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin:/root/.go/bin:/root/go/bin"
ENV SHELL=/bin/bash
ENV GOROOT=/root/.go
ENV GOPATH=/root/go
ENV START_AGENT_ALLOWED=true

WORKDIR /home/root/bootstrap/
COPY agent-conductor/agent-fargate/container/inside-container/work/ /home/root/work/
COPY agent-conductor/agent-fargate/container/inside-container/bootstrap/ ./
COPY agent-conductor/agent-fargate/container/inside-container/bootstrap/entrypoint.sh /entrypoint.sh

# directory structure
RUN mkdir /tmp/agent-meta/



# separating these installs here helps with dokcer build stage caching
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs npm \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# install codex + runtime deps, and general runtime stuff
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git ncurses-bin tar tmux \
    && npm install -g @openai/codex@0.128.0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# install software for codex to use
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        fd-find \
        file \
        jq \
        less \
        make \
        procps \
        psmisc \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        tree \
        unzip \
        zip \
        fd-find \
        vim \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# install development tools
RUN bash ./go.sh --version 1.26.2
RUN chmod +x /entrypoint.sh

COPY --from=build /out/codex-wrapper /usr/local/bin/codex-wrapper
COPY --from=build /out/get-secrets /usr/local/bin/get-secrets

WORKDIR /home/root/work/

ENTRYPOINT ["/entrypoint.sh"]
