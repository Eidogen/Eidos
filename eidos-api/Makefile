.PHONY: build run test clean

SERVICE_NAME := eidos-api
HTTP_PORT := 8080

build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go

run:
	go run ./cmd/main.go

test:
	go test -v ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

docker-build:
	docker build -t $(SERVICE_NAME):latest -f Dockerfile ..
