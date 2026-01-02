.PHONY: build run test clean proto

SERVICE_NAME := eidos-matching
GRPC_PORT := 50052

build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go

run:
	go run ./cmd/main.go

test:
	go test -v ./...

test-cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

proto:
	$(MAKE) -C .. proto-matching

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

docker-build:
	docker build -t $(SERVICE_NAME):latest -f Dockerfile ..

docker-run:
	docker run -p $(GRPC_PORT):$(GRPC_PORT) $(SERVICE_NAME):latest
