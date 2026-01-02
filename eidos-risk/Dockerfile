FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum* ./
COPY ../eidos-common/go.mod ../eidos-common/
RUN go mod download
COPY . .
COPY ../eidos-common ../eidos-common
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/service /app/service
COPY config/config.yaml /app/config/config.yaml
EXPOSE 50055 8080
CMD ["/app/service"]
