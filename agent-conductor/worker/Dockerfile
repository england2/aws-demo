# ==============================================================
# build stage
# ==============================================================
FROM golang:1.26-alpine AS build

RUN apk add --no-cache fish protobuf protobuf-dev && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

COPY scripts/gen-proto /src/scripts/gen-proto
COPY src/proto/ /src/src/proto/
COPY src/sharedlib/ /src/src/sharedlib/

RUN /src/scripts/gen-proto

WORKDIR /src/src/worker/go-worker

COPY src/worker/go-worker/go.mod src/worker/go-worker/go.sum ./
COPY src/worker/go-worker/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker .

# ==============================================================
# runtime stage
# ==============================================================
FROM debian:bookworm-slim

ENV PATH="/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin:/root/.go/bin:/root/go/bin"
ENV SHELL=/bin/bash
ENV GOROOT=/root/.go
ENV GOPATH=/root/go
ENV START_AGENT_ALLOWED=true

WORKDIR /worker/work

RUN mkdir -p /tmp/agent-meta /worker/work && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        bubblewrap \
        ca-certificates \
        curl \
        fd-find \
        file \
        gh \
        git \
        jq \
        less \
        make \
        ncurses-bin \
        nodejs \
        npm \
        openssh-server \
        procps \
        psmisc \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        tar \
        tmux \
        tree \
        unzip \
        vim \
        zip && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN npm install -g @openai/codex@0.128.0

COPY --from=build /out/worker /usr/local/bin/worker
# Doesn't dropping in a `.codex` doesn't fix EOF sdk init error.
# COPY src/worker/docker-resources/.codex/ /root/.codex/
COPY src/worker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# real
# ENTRYPOINT ["/usr/local/bin/worker"]

# testing 1
ENTRYPOINT ["/entrypoint.sh"]
# testing 2

# ENTRYPOINT ["sleep infinity"]
