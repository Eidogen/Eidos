.PHONY: build run test clean proto

SERVICE_NAME := eidos-market
GRPC_PORT := 50053

build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go

run:
	go run ./cmd/main.go

test:
	go test -v ./...

clean:
	rm -rf bin/

proto:
	$(MAKE) -C .. proto-market

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

docker-build:
	docker build -t $(SERVICE_NAME):latest -f Dockerfile ..
