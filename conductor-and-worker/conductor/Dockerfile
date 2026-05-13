# ==============================================================
# build stage
# ==============================================================
FROM golang:1.26-alpine AS build

WORKDIR /src/conductor-and-worker/conductor/go-conductor

COPY conductor-and-worker/proto/ /src/conductor-and-worker/proto/
COPY conductor-and-worker/sharedlib/ /src/conductor-and-worker/sharedlib/
COPY conductor-and-worker/conductor/go-conductor/go.mod conductor-and-worker/conductor/go-conductor/go.sum ./
COPY conductor-and-worker/conductor/go-conductor/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/conductor .

# ==============================================================
# runtime stage
# ==============================================================
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl sqlite3 tmux zip unzip && \
    curl -sSf https://atlasgo.sh | sh && \
    mkdir -p /conductor /usr/local/share/conductor && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /out/conductor /usr/local/bin/conductor
COPY conductor-and-worker/conductor/go-conductor/worker-resources/ /usr/local/share/conductor/worker-resources/
COPY conductor-and-worker/conductor/go-conductor/db-sqlc/database.sql /usr/local/share/conductor/database.sql
COPY conductor-and-worker/conductor/docker-entrypoint.sh /usr/local/bin/conductor-entrypoint

WORKDIR /conductor

RUN chmod +x /usr/local/bin/conductor-entrypoint

ENTRYPOINT ["/usr/local/bin/conductor-entrypoint"]
CMD ["-addr=0.0.0.0:50055", "-test-db-loc=/conductor/data/scheduler.sqlite"]
