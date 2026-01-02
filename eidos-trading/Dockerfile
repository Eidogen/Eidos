# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum* ./
COPY ../eidos-common/go.mod ../eidos-common/

# Download dependencies
RUN go mod download

# Copy source
COPY . .
COPY ../eidos-common ../eidos-common

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/service /app/service
COPY config/config.yaml /app/config/config.yaml

EXPOSE 50051 8080

CMD ["/app/service"]
