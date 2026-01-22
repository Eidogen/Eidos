FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY eidos-matching/go.mod eidos-matching/go.sum* ./eidos-matching/
COPY eidos-common/go.mod eidos-common/
COPY proto/go.mod proto/
RUN cd eidos-matching && go mod download
COPY eidos-matching/ ./eidos-matching/
COPY eidos-common ./eidos-common
COPY proto ./proto
WORKDIR /app/eidos-matching
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/service /app/service
COPY eidos-matching/config/config.yaml /app/config/config.yaml
EXPOSE 50052 8080
CMD ["/app/service"]
