FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY eidos-market/go.mod eidos-market/go.sum* ./eidos-market/
COPY eidos-common/go.mod eidos-common/
COPY proto/go.mod proto/
RUN cd eidos-market && go mod download
COPY eidos-market/ ./eidos-market/
COPY eidos-common ./eidos-common
COPY proto ./proto
WORKDIR /app/eidos-market
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/service /app/service
COPY eidos-market/config/config.yaml /app/config/config.yaml
EXPOSE 50053 8080
CMD ["/app/service"]
