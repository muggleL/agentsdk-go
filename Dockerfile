# syntax=docker/dockerfile:1
ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-alpine AS builder
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/agentctl ./cmd/agentctl

FROM alpine:3.20
RUN addgroup -S agent && adduser -S agent -G agent \
    && apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=builder /out/agentctl /usr/local/bin/agentctl
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
RUN mkdir -p /var/agentsdk/sessions && chown -R agent:agent /var/agentsdk
ENV PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:${PORT}/health || exit 1
USER agent
ENTRYPOINT ["/entrypoint.sh"]
