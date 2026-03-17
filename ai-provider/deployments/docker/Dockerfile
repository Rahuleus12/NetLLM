# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -o ai-provider \
    ./cmd/server

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 aiuser && \
    adduser -D -u 1000 -G aiuser aiuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/ai-provider .

# Copy configuration files
COPY --chown=aiuser:aiuser configs/ ./configs/

# Create necessary directories
RUN mkdir -p /var/log/ai-provider /models /tmp/ai-provider && \
    chown -R aiuser:aiuser /var/log/ai-provider /models /tmp/ai-provider

# Switch to non-root user
USER aiuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set environment variables
ENV AI_PROVIDER_SYSTEM_HOST=0.0.0.0 \
    AI_PROVIDER_SYSTEM_PORT=8080

# Run the binary
ENTRYPOINT ["./ai-provider"]
CMD ["--config", "./configs/config.yaml"]
