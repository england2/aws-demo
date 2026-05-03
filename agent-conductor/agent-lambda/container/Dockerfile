# syntax=docker/dockerfile:1

ARG TARGETARCH

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETARCH

WORKDIR /src

COPY go-codex-wrapper/go.mod ./
COPY go-codex-wrapper/*.go ./
COPY go-codex-wrapper/tools/ ./tools/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o /out/codex-wrapper .

FROM debian:bookworm-slim

ENV SHELL=/bin/bash
ENV GOROOT=/root/.go
ENV GOPATH=/root/go
ENV PATH="/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin:/root/.go/bin:/root/go/bin"

WORKDIR /home/root/bootstrap/
COPY inside-container/bootstrap/ ./
COPY inside-container/work/ /home/root/work/

# install codex and runtime deps
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git ncurses-bin nodejs npm tar \
    && npm install -g @openai/codex@0.128.0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# install development tools
RUN bash ./go.sh --version 1.26.2

COPY --from=build /out/codex-wrapper /usr/local/bin/codex-wrapper

ENTRYPOINT ["/usr/local/bin/codex-wrapper"]
