FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY eidos-admin/go.mod eidos-admin/go.sum* ./eidos-admin/
COPY eidos-common/go.mod eidos-common/
COPY proto/go.mod proto/
RUN cd eidos-admin && go mod download
COPY eidos-admin/ ./eidos-admin/
COPY eidos-common ./eidos-common
COPY proto ./proto
WORKDIR /app/eidos-admin
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/service /app/service
COPY eidos-admin/config/config.yaml /app/config/config.yaml
EXPOSE 8088
CMD ["/app/service"]
