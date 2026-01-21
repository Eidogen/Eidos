# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY eidos-trading/go.mod eidos-trading/go.sum* ./eidos-trading/
COPY eidos-common/go.mod eidos-common/
COPY proto/go.mod proto/
RUN cd eidos-trading && go mod download
COPY eidos-trading/ ./eidos-trading/
COPY eidos-common ./eidos-common
COPY proto ./proto
WORKDIR /app/eidos-trading
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/service /app/service
COPY eidos-trading/config/config.yaml /app/config/config.yaml

EXPOSE 50051 8080

CMD ["/app/service"]
