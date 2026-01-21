FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY eidos-risk/go.mod eidos-risk/go.sum* ./eidos-risk/
COPY eidos-common/go.mod eidos-common/
COPY proto/go.mod proto/
RUN cd eidos-risk && go mod download
COPY eidos-risk/ ./eidos-risk/
COPY eidos-common ./eidos-common
COPY proto ./proto
WORKDIR /app/eidos-risk
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/service /app/service
COPY eidos-risk/config/config.yaml /app/config/config.yaml
EXPOSE 50055 8080
CMD ["/app/service"]
