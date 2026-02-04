# G0 CMS - Lightweight CMS
# Multi-stage build for minimal image size

# === BUILD STAGE ===
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install git for go mod download (some deps need it)
RUN apk add --no-cache git

# Copy go mod files first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o g0 .

# === RUNTIME STAGE ===
FROM alpine:3.19

# Add ca-certificates for HTTPS (OAuth needs it)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -h /app appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/g0 /app/g0

# Create data directory (config + database)
# Mount this as a volume to persist data
RUN mkdir -p /app/data && chown -R appuser:appuser /app

# Volume for persistent data (config.yaml + database)
VOLUME /app/data

# Switch to non-root user
USER appuser

# Expose port (default 8080, configurable via config.yaml)
EXPOSE 8080

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/ || exit 1

# Run
CMD ["/app/g0"]
