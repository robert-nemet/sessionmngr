FROM golang:1.25-alpine AS builder
ARG VERSION=dev
ARG BUILD_TIME=unknown
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/robert-nemet/sessionmngr/internal/version.Version=${VERSION} -X github.com/robert-nemet/sessionmngr/internal/version.BuildTimestamp=${BUILD_TIME}" \
    -o /session-manager-mcp ./cmd

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /session-manager-mcp /session-manager-mcp
ENTRYPOINT ["/session-manager-mcp"]
