# ==============================================================
# build stage
# ==============================================================
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src/conductor-and-worker/worker/go-worker

COPY conductor-and-worker/proto/ /src/conductor-and-worker/proto/
COPY conductor-and-worker/sharedlib/ /src/conductor-and-worker/sharedlib/
COPY conductor-and-worker/worker/go-worker/go.mod conductor-and-worker/worker/go-worker/go.sum ./
RUN go mod download

COPY conductor-and-worker/worker/go-worker/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/worker .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/get-secrets ./cmd/get-secrets
RUN GOBIN=/out CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go install github.com/benhoyt/goawk@v1.31.0

# ==============================================================
# runtime stage
# ==============================================================
FROM debian:bookworm-slim

ENV PATH="/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin:/root/.go/bin:/root/go/bin"
ENV SHELL=/bin/bash
ENV TERM=xterm
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
        fish \
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
COPY --from=build /out/get-secrets /usr/local/bin/get-secrets
COPY --from=build /out/goawk /usr/local/bin/goawk
# Doesn't dropping in a `.codex` doesn't fix EOF sdk init error.
# COPY conductor-and-worker/worker/docker-resources/.codex/ /root/.codex/
COPY conductor-and-worker/worker/docker-resources/inside/go.sh /usr/local/bin/go.sh
COPY conductor-and-worker/worker/docker-resources/inside/entrypoint.sh /entrypoint.sh
COPY conductor-and-worker/worker/docker-resources/inside/clone-repo.fish /usr/local/bin/clone-repo.fish

RUN chmod +x /entrypoint.sh /usr/local/bin/clone-repo.fish /usr/local/bin/go.sh && \
    /usr/local/bin/go.sh && \
    go version

ENTRYPOINT ["/entrypoint.sh"]
