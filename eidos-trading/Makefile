.PHONY: build run test clean proto

SERVICE_NAME := eidos-trading
GRPC_PORT := 50051

# Build
build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go

# Run locally
run:
	go run ./cmd/main.go

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Generate proto (uses root Makefile)
proto:
	$(MAKE) -C .. proto-trading

# Lint
lint:
	golangci-lint run ./...

# Format
fmt:
	go fmt ./...

# Docker build
docker-build:
	docker build -t $(SERVICE_NAME):latest -f Dockerfile ..

# Docker run
docker-run:
	docker run -p $(GRPC_PORT):$(GRPC_PORT) $(SERVICE_NAME):latest
