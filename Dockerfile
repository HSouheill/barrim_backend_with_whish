# Development Dockerfile for Barrim Backend
FROM golang:1.23.1-alpine AS development

# Install development tools and dependencies
RUN apk --no-cache add \
    ca-certificates \
    curl \
    git \
    make \
    bash \
    tzdata \
    redis \
    mongodb-tools

# Install air for hot reloading (compatible with Go 1.23)
RUN go install github.com/cosmtrek/air@v1.49.0

# Create app directory
WORKDIR /app

# Create non-root user for development
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Create upload directories with proper permissions
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

# Switch to non-root user
USER appuser

# Expose port 8080
EXPOSE 8080

# Use air for hot reloading in development
CMD ["air", "-c", ".air.toml"]
