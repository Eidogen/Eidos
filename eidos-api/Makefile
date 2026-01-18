.PHONY: build run test test-unit test-integration test-race test-coverage clean lint fmt docker-build

SERVICE_NAME := eidos-api
HTTP_PORT := 8080
COVERAGE_FILE := coverage.out

# 构建
build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go

# 运行
run:
	go run ./cmd/main.go

# 测试相关
test: test-unit test-integration

test-unit:
	go test -v -race ./internal/...

test-integration:
	go test -v -race -tags=integration ./test/integration/...

test-race:
	go test -v -race ./internal/... ./test/...

test-coverage:
	go test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./internal/...
	go tool cover -func=$(COVERAGE_FILE) | tail -20

test-coverage-html: test-coverage
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-all: test-coverage test-integration
	@echo "All tests completed"

# 代码质量
lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

# 清理
clean:
	rm -rf bin/
	rm -f $(COVERAGE_FILE) coverage.html

# Docker
docker-build:
	docker build -t $(SERVICE_NAME):latest -f Dockerfile ..

docker-run:
	docker run -p $(HTTP_PORT):$(HTTP_PORT) $(SERVICE_NAME):latest

# 开发辅助
deps:
	go mod download
	go mod tidy

proto:
	protoc --go_out=. --go-grpc_out=. api/proto/**/*.proto

mock:
	mockgen -source=internal/service/order_service.go -destination=test/mock/order_service_mock.go -package=mock
	mockgen -source=internal/service/balance_service.go -destination=test/mock/balance_service_mock.go -package=mock

# 帮助
help:
	@echo "Available targets:"
	@echo "  build             - Build the binary"
	@echo "  run               - Run the service"
	@echo "  test              - Run all tests (unit + integration)"
	@echo "  test-unit         - Run unit tests only"
	@echo "  test-integration  - Run integration tests only"
	@echo "  test-race         - Run tests with race detector"
	@echo "  test-coverage     - Run tests with coverage report"
	@echo "  test-coverage-html- Generate HTML coverage report"
	@echo "  lint              - Run linter"
	@echo "  fmt               - Format code"
	@echo "  clean             - Clean build artifacts"
	@echo "  docker-build      - Build Docker image"
	@echo "  docker-run        - Run Docker container"
	@echo "  deps              - Download and tidy dependencies"
	@echo "  mock              - Generate mocks"
