# Build stage
FROM golang:1.25.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Install sqlc if not present (optional, can be pre-installed)
# RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Generate sqlc code (if needed, uncomment)
# RUN cd internal/services/media-service/repository && sqlc generate
# RUN cd internal/services/access-service/repository && sqlc generate

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bin/lovebin ./cmd/lovebin

# Runtime stage
FROM alpine:latest

# Install ca-certificates and wget for HTTPS requests and healthcheck
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/lovebin .

# Copy frontend directory
COPY --from=builder /app/frontend ./frontend

# Change ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./lovebin"]

