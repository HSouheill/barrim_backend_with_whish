# Build stage
FROM golang:1.23.1-alpine AS builder

# Install build dependencies
RUN apk --no-cache add git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o barrim_backend .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests, curl for health checks, and redis-cli for debugging
RUN apk --no-cache add ca-certificates curl tzdata redis

WORKDIR /app

# Create non-root user
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001

# Copy binary from builder stage
COPY --from=builder /build/barrim_backend ./barrim_backend

# Firebase credentials will be provided via environment variables

# Create upload directories
RUN mkdir -p /app/uploads/bookings \
    /app/uploads/category \
    /app/uploads/certificates \
    /app/uploads/companies \
    /app/uploads/logo \
    /app/uploads/logos \
    /app/uploads/videos \
    /app/uploads/profiles \
    /app/uploads/serviceprovider \
    /app/uploads/vouchers

# Change ownership to non-root user
RUN chown -R appuser:appuser /app

USER appuser

# Expose port 8080
EXPOSE 8080

CMD ["./barrim_backend"]
