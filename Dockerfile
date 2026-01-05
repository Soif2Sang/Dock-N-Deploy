# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies (needed for CGO and some packages)
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled for certain dependencies
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/cicd-backend .

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install necessary runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    docker-cli \
    git \
    tzdata

# Copy the binary from builder
COPY --from=builder /app/cicd-backend .

# Copy config files if needed
COPY schema.sql .
COPY dummy-.gitlab-ci.yml .
COPY dummy2-.gitlab-ci.yml .

# Create directory for pipeline workspaces
RUN mkdir -p /tmp/cicd-workspaces

# Expose API port
EXPOSE 8080

# Run the application
CMD ["./cicd-backend"]