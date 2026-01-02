.PHONY: build run test clean proto

SERVICE_NAME := eidos-admin
HTTP_PORT := 8088

build:
	go build -o bin/$(SERVICE_NAME) ./cmd/main.go

run:
	go run ./cmd/main.go

test:
	go test -v ./...

clean:
	rm -rf bin/

proto:
	$(MAKE) -C .. proto-admin

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

docker-build:
	docker build -t $(SERVICE_NAME):latest -f Dockerfile ..
