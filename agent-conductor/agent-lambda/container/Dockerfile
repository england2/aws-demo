# ai--done
# use a multi-stage build to compile the golang program and then make a debian container that starts the golang entrypoint
FROM golang:1.26-bookworm AS build

WORKDIR /src

COPY go-codex-wrapper/go.mod ./
COPY go-codex-wrapper/*.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/codex-wrapper .

FROM debian:bookworm-slim

COPY --from=build /out/codex-wrapper /usr/local/bin/codex-wrapper

ENTRYPOINT ["/usr/local/bin/codex-wrapper"]
