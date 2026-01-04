.PHONY: build run test clean proto migrate-up migrate-down migrate-status migrate-create

SERVICE_NAME := eidos-trading
GRPC_PORT := 50051
MIGRATE_DIR := migrations

# Build
build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go
	go build -o bin/migrate ./cmd/migrate/main.go

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

# Database migrations
migrate-up:
	go run ./cmd/migrate/main.go -cmd up -dir $(MIGRATE_DIR)

migrate-down:
	go run ./cmd/migrate/main.go -cmd down -dir $(MIGRATE_DIR)

migrate-status:
	go run ./cmd/migrate/main.go -cmd status -dir $(MIGRATE_DIR)

migrate-create:
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=<migration_name>"; exit 1; fi
	go run ./cmd/migrate/main.go -cmd create -dir $(MIGRATE_DIR) $(name)
