# syntax=docker/dockerfile:1

# ── Stage 1: build the Go binary ─────────────────────────────────────────────
FROM golang:1.26-alpine AS builder
WORKDIR /src

# Cache dependencies separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /collector \
    ./cmd/collector

# ── Stage 2: minimal runtime image ───────────────────────────────────────────
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /collector /app/collector

# Collector listens on 4318 (HTTP ingest + query)
EXPOSE 4318

# Run as non-root
RUN addgroup -S tracing && adduser -S -G tracing tracing
USER tracing

ENTRYPOINT ["/app/collector"]
