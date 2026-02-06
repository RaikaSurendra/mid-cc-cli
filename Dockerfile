# Multi-stage build for Claude Terminal Service

# Stage 1: Build
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN make build

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies + Node.js for Claude Code CLI
RUN apk add --no-cache \
    ca-certificates \
    bash \
    curl \
    nodejs \
    npm \
    git

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Create app user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Create necessary directories
RUN mkdir -p /app /tmp/claude-sessions /var/log && \
    chown -R appuser:appgroup /app /tmp/claude-sessions /var/log

# Copy binaries from builder
COPY --from=builder /app/bin/claude-terminal-service /app/
COPY --from=builder /app/bin/ecc-poller /app/

# Copy configuration
COPY --chown=appuser:appgroup .env.example /app/.env.example

# Switch to app user
USER appuser

WORKDIR /app

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# Default command
CMD ["./claude-terminal-service"]
