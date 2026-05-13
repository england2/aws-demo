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

WORKDIR /src/src/conductor/go-conductor

COPY src/conductor/go-conductor/go.mod src/conductor/go-conductor/go.sum ./
COPY src/conductor/go-conductor/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/conductor .

# ==============================================================
# runtime stage
# ==============================================================
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends zip unzip && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /out/conductor /usr/local/bin/conductor
COPY src/conductor/go-conductor/worker-resources/ /usr/local/bin/worker-resources/

WORKDIR /usr/local/bin

ENTRYPOINT ["/usr/local/bin/conductor"]
