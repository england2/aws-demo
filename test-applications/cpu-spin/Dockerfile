# build go image
FROM --platform=linux/amd64 golang:1-bookworm AS builder

WORKDIR /build

COPY spin/go.mod ./spin/go.mod
WORKDIR /build/spin
RUN go mod download

COPY spin/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /build/cpu-spin .

# runtime
FROM --platform=linux/amd64 debian:bookworm-slim

WORKDIR /app

COPY --from=builder /build/cpu-spin /usr/local/bin/cpu-spin

CMD ["cpu-spin"]
