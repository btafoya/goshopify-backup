# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o goshopify-backup .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates tzdata && \
    rm -rf /var/cache/ap/*

# Create non-root user
RUN addgroup -g 65534 -S shopify-backup && \
    adduser -u 65534 -S shopify-backup -G shopify-backup

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/goshopify-backup .

# Create backup directory
RUN mkdir -p /backups && chown -R shopify-backup:shopify-backup /backups /app

# Switch to non-root user
USER shopify-backup

# Set resource limits
# These can be overridden at runtime
ENV GOMAXPROCS=1

# Health check
HEALTHCHECK --interval=5m --timeout=30s --start-period=2m --retries=3 \
    CMD ["/app/goshopify-backup", "--health-check"]

ENTRYPOINT ["/app/goshopify-backup"]