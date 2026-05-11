# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
COPY pkg/auth/ ./pkg/auth/
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o goshopify-backup .

# Runtime stage — switched to node:20-alpine so @shopify/cli is available
# in $PATH for the themes backup module.
FROM node:20-alpine

# SHOPIFY_CLI_VERSION pins the Shopify CLI release baked into the image.
# Override at build time: --build-arg SHOPIFY_CLI_VERSION=@shopify/cli@3.65.0
ARG SHOPIFY_CLI_VERSION=@shopify/cli@3

# Install ca-certificates for HTTPS requests + Shopify CLI
RUN apk add --no-cache ca-certificates tzdata && \
    npm install -g "${SHOPIFY_CLI_VERSION}" && \
    npm cache clean --force && \
    rm -rf /var/cache/apk/* /tmp/*

# Create non-root user (node:20-alpine already has 'node' uid 1000, use distinct uid 65534)
RUN addgroup -g 65534 -S shopify-backup && \
    adduser -u 65534 -S shopify-backup -G shopify-backup

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/goshopify-backup .

# Create backup + credentials directories
RUN mkdir -p /backups /credentials && \
    chown -R shopify-backup:shopify-backup /backups /credentials /app

# Switch to non-root user
USER shopify-backup

# Resource limits (overridable at runtime)
ENV GOMAXPROCS=1

# Health check
HEALTHCHECK --interval=5m --timeout=30s --start-period=2m --retries=3 \
    CMD ["/app/goshopify-backup", "--health-check"]

ENTRYPOINT ["/app/goshopify-backup"]
