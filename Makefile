.PHONY: all build test clean proto infra-up infra-down help

# Service list
SERVICES := eidos-api eidos-trading eidos-matching eidos-market eidos-chain eidos-risk eidos-jobs eidos-admin

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOFMT := $(GOCMD) fmt
GOMOD := $(GOCMD) mod

# Proto parameters
PROTOC := protoc
PROTO_DIR := proto
PROTO_GO_OUT := --go_out=. --go_opt=paths=source_relative
PROTO_GRPC_OUT := --go-grpc_out=. --go-grpc_opt=paths=source_relative

# Docker Compose files
DC_INFRA := docker-compose.infra.yml
DC_SERVICES := docker-compose.services.yml
DC_LEGACY := docker-compose.yml

# Default target
all: proto build

# ========================================
# Help
# ========================================
help:
	@echo "Eidos Trading System - Makefile Guide"
	@echo ""
	@echo "Quick Start:"
	@echo "  make up                 Start everything (infra + services)"
	@echo "  make down               Stop everything"
	@echo "  make ps                 Show running containers"
	@echo "  make logs               View all logs"
	@echo ""
	@echo "Basic Commands:"
	@echo "  make build              Build all services"
	@echo "  make test               Run all unit tests"
	@echo "  make clean              Clean build artifacts"
	@echo "  make fmt                Format code"
	@echo "  make lint               Run code linting"
	@echo ""
	@echo "Proto:"
	@echo "  make proto              Generate all proto files"
	@echo "  make proto-trading      Generate trading proto"
	@echo "  make proto-matching     Generate matching proto"
	@echo ""
	@echo "Infrastructure (docker-compose.infra.yml):"
	@echo "  make infra-up           Start infrastructure (DB, Redis, Kafka, Nacos, etc.)"
	@echo "  make infra-down         Stop infrastructure (data preserved)"
	@echo "  make infra-clean        Stop and delete all data (CAUTION!)"
	@echo "  make infra-logs         View all infrastructure logs"
	@echo "  make infra-logs-db      View database logs"
	@echo "  make infra-logs-mq      View Kafka logs"
	@echo "  make infra-logs-nacos   View Nacos logs"
	@echo "  make infra-status       View infrastructure status"
	@echo "  make infra-kafka-ui     Start Kafka UI (debug)"
	@echo "  make infra-restart      Restart infrastructure"
	@echo ""
	@echo "Services (docker-compose.services.yml):"
	@echo "  make services-up        Start all application services"
	@echo "  make services-down      Stop all application services"
	@echo "  make services-logs      View all services logs"
	@echo "  make services-logs-<x>  View logs for eidos-<x> (e.g., services-logs-trading)"
	@echo "  make services-restart   Restart all services"
	@echo "  make services-restart-<x>  Restart eidos-<x>"
	@echo "  make services-status    View services status"
	@echo "  make services-core      Start core services only (trading + matching)"
	@echo "  make services-all       Start all services including API"
	@echo ""
	@echo "Single Service:"
	@echo "  make build-api          Build eidos-api"
	@echo "  make run-api            Run eidos-api locally"
	@echo "  make test-api           Test eidos-api"
	@echo ""
	@echo "Integration Tests:"
	@echo "  make test-integration   Run all integration tests"
	@echo "  make test-order-flow    Test order flow"
	@echo "  make test-deposit-flow  Test deposit flow"
	@echo "  make test-market-flow   Test market data flow"
	@echo ""
	@echo "Database (migrations run automatically on service startup):"
	@echo "  make db-setup           Create DBs and seed test data"
	@echo "  make db-seed            Load test data into databases"
	@echo "  make db-reset           Reset databases (DESTROYS ALL DATA!)"
	@echo "  make db-create          Create databases if they don't exist"
	@echo ""
	@echo "Development:"
	@echo "  make dev-up             Start infrastructure for local dev"
	@echo "  make dev-down           Stop all containers"
	@echo "  make dev-local          Start infra with local connection info"
	@echo "  make health-check       Check health of all services"
	@echo "  make verify-data-flow   Verify data flow between services"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build       Build all Docker images"
	@echo "  make docker-push        Push all Docker images"
	@echo ""
	@echo "Endpoints:"
	@echo "  PostgreSQL:   localhost:5432"
	@echo "  TimescaleDB:  localhost:5433"
	@echo "  Redis:        localhost:6379"
	@echo "  Kafka:        localhost:29092 (external)"
	@echo "  Nacos:        http://localhost:8848/nacos"
	@echo "  Prometheus:   http://localhost:9090"
	@echo "  Grafana:      http://localhost:3000"
	@echo "  API Gateway:  http://localhost:8080"
	@echo "  Admin:        http://localhost:8088"

# ========================================
# Build
# ========================================
build: $(addprefix build-,$(SERVICES))

build-%:
	@echo "Building $*..."
	@cd $* && $(GOBUILD) -o bin/$* ./cmd/main.go

# ========================================
# Unit Tests
# ========================================
test: $(addprefix test-,$(SERVICES))
	@cd eidos-common && $(GOTEST) -v ./...

test-%:
	@echo "Testing $*..."
	@cd $* && $(GOTEST) -v ./...

test-cover:
	@for svc in $(SERVICES); do \
		echo "Testing $$svc with coverage..."; \
		cd $$svc && $(GOTEST) -v -coverprofile=coverage.out ./... && cd ..; \
	done

test-cover-html: test-cover
	@for svc in $(SERVICES); do \
		if [ -f $$svc/coverage.out ]; then \
			cd $$svc && $(GOCMD) tool cover -html=coverage.out -o coverage.html && cd ..; \
		fi \
	done
	@echo "Coverage reports generated!"

# ========================================
# Integration Tests
# ========================================
test-integration:
	@echo "Running all integration tests..."
	@cd tests/integration && $(GOTEST) -v -timeout 10m ./...

test-order-flow:
	@echo "Running order flow integration tests..."
	@cd tests/integration && $(GOTEST) -v -timeout 5m -run TestOrderFlowSuite ./...

test-deposit-flow:
	@echo "Running deposit flow integration tests..."
	@cd tests/integration && $(GOTEST) -v -timeout 5m -run TestDepositFlowSuite ./...

test-withdrawal-flow:
	@echo "Running withdrawal flow integration tests..."
	@cd tests/integration && $(GOTEST) -v -timeout 5m -run TestWithdrawalFlowSuite ./...

test-market-flow:
	@echo "Running market data flow integration tests..."
	@cd tests/integration && $(GOTEST) -v -timeout 5m -run TestMarketDataFlowSuite ./...

# ========================================
# Code Quality
# ========================================
fmt:
	@for svc in $(SERVICES) eidos-common; do \
		echo "Formatting $$svc..."; \
		cd $$svc && $(GOFMT) ./... && cd ..; \
	done

lint:
	@for svc in $(SERVICES) eidos-common; do \
		echo "Linting $$svc..."; \
		cd $$svc && golangci-lint run ./... && cd ..; \
	done

vet:
	@for svc in $(SERVICES) eidos-common; do \
		echo "Vetting $$svc..."; \
		cd $$svc && $(GOCMD) vet ./... && cd ..; \
	done

# ========================================
# Proto Generation
# ========================================
proto: proto-clean proto-common proto-kafka proto-trading proto-matching proto-market proto-chain proto-risk proto-jobs proto-admin proto-settlement
	@echo "All proto files generated successfully!"

proto-clean:
	@echo "Cleaning old generated files..."
	@find proto -name "*.pb.go" -delete 2>/dev/null || true

proto-common:
	@echo "Generating common proto..."
	@$(PROTOC) \
		--go_out=. \
		--go_opt=module=github.com/eidos-exchange/eidos \
		-I /opt/homebrew/include \
		-I $(PROTO_DIR) \
		$(PROTO_DIR)/common/enums.proto \
		$(PROTO_DIR)/common/pagination.proto \
		$(PROTO_DIR)/common/response.proto
	@echo "Common proto generated!"

proto-kafka:
	@echo "Generating kafka proto..."
	@if [ -f $(PROTO_DIR)/common/kafka.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/common/kafka.proto; \
		echo "Kafka proto generated!"; \
	else \
		echo "Kafka proto not found, skipping..."; \
	fi

proto-trading:
	@echo "Generating trading proto..."
	@$(PROTOC) \
		--go_out=. \
		--go_opt=module=github.com/eidos-exchange/eidos \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/eidos-exchange/eidos \
		-I /opt/homebrew/include \
		-I $(PROTO_DIR) \
		$(PROTO_DIR)/trading/v1/trading.proto
	@echo "Trading proto generated!"

proto-matching:
	@echo "Generating matching proto..."
	@if [ -f $(PROTO_DIR)/matching/v1/matching.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/matching/v1/matching.proto; \
		echo "Matching proto generated!"; \
	else \
		echo "Matching proto not found, skipping..."; \
	fi

proto-market:
	@echo "Generating market proto..."
	@if [ -f $(PROTO_DIR)/market/v1/market_service.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/market/v1/market_service.proto; \
		echo "Market proto generated!"; \
	else \
		echo "Market proto not found, skipping..."; \
	fi

proto-chain:
	@echo "Generating chain proto..."
	@if [ -f $(PROTO_DIR)/chain/v1/chain.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/chain/v1/chain.proto; \
		echo "Chain proto generated!"; \
	else \
		echo "Chain proto not found, skipping..."; \
	fi

proto-risk:
	@echo "Generating risk proto..."
	@if [ -f $(PROTO_DIR)/risk/v1/risk.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/risk/v1/risk.proto; \
		echo "Risk proto generated!"; \
	else \
		echo "Risk proto not found, skipping..."; \
	fi

proto-jobs:
	@echo "Generating jobs proto..."
	@if [ -f $(PROTO_DIR)/jobs/v1/jobs.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/jobs/v1/jobs.proto; \
		echo "Jobs proto generated!"; \
	else \
		echo "Jobs proto not found, skipping..."; \
	fi

proto-admin:
	@echo "Generating admin proto..."
	@if [ -f $(PROTO_DIR)/admin/v1/admin.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/admin/v1/admin.proto; \
		echo "Admin proto generated!"; \
	else \
		echo "Admin proto not found, skipping..."; \
	fi

proto-settlement:
	@echo "Generating settlement proto..."
	@if [ -f $(PROTO_DIR)/settlement/v1/settlement.proto ]; then \
		$(PROTOC) \
			--go_out=. \
			--go_opt=module=github.com/eidos-exchange/eidos \
			--go-grpc_out=. \
			--go-grpc_opt=module=github.com/eidos-exchange/eidos \
			-I /opt/homebrew/include \
			-I $(PROTO_DIR) \
			$(PROTO_DIR)/settlement/v1/settlement.proto; \
		echo "Settlement proto generated!"; \
	else \
		echo "Settlement proto not found, skipping..."; \
	fi

# ========================================
# Infrastructure (New - Separated)
# ========================================
infra-up:
	@echo "Starting infrastructure components..."
	docker-compose -f $(DC_INFRA) up -d
	@echo "Waiting for services to be ready..."
	@sleep 15
	@echo ""
	@echo "Infrastructure is ready!"
	@echo "======================================"
	@echo "  PostgreSQL:   localhost:5432"
	@echo "  TimescaleDB:  localhost:5433"
	@echo "  Redis:        localhost:6379"
	@echo "  Kafka:        localhost:29092 (external) / kafka:9092 (internal)"
	@echo "  Nacos:        http://localhost:8848/nacos (nacos/nacos)"
	@echo "  Prometheus:   http://localhost:9090"
	@echo "  Grafana:      http://localhost:3000 (admin/admin123)"
	@echo "======================================"

infra-down:
	@echo "Stopping infrastructure (data preserved in volumes)..."
	docker-compose -f $(DC_INFRA) down
	@echo "Infrastructure stopped. Data is preserved in Docker volumes."

infra-logs:
	docker-compose -f $(DC_INFRA) logs -f

infra-logs-db:
	docker-compose -f $(DC_INFRA) logs -f postgres timescaledb

infra-logs-mq:
	docker-compose -f $(DC_INFRA) logs -f kafka

infra-logs-nacos:
	docker-compose -f $(DC_INFRA) logs -f nacos

infra-clean:
	@echo "WARNING: This will delete all data in Docker volumes!"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	docker-compose -f $(DC_INFRA) down -v
	@echo "Infrastructure stopped and all data deleted."

infra-status:
	docker-compose -f $(DC_INFRA) ps

infra-kafka-ui:
	@echo "Starting Kafka UI..."
	docker-compose -f $(DC_INFRA) --profile debug up -d kafka-ui
	@echo "Kafka UI available at http://localhost:8090"

infra-restart:
	@echo "Restarting infrastructure..."
	docker-compose -f $(DC_INFRA) restart
	@echo "Infrastructure restarted."

# ========================================
# Application Services (New - Separated)
# ========================================
services-up:
	@echo "Starting all application services..."
	@echo "(Make sure infrastructure is running: make infra-up)"
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) up -d
	@echo ""
	@echo "Services started!"
	@echo "======================================"
	@echo "  API Gateway:     http://localhost:8080"
	@echo "  Admin:           http://localhost:8088"
	@echo "  Trading (gRPC):  localhost:50051 (HTTP: 8081)"
	@echo "  Matching (gRPC): localhost:50052 (HTTP: 8082)"
	@echo "  Market (gRPC):   localhost:50053 (HTTP: 8083)"
	@echo "  Chain (gRPC):    localhost:50054 (HTTP: 8084)"
	@echo "  Risk (gRPC):     localhost:50055 (HTTP: 8085)"
	@echo "  Jobs (gRPC):     localhost:50056 (HTTP: 8086)"
	@echo "======================================"

services-down:
	@echo "Stopping all application services..."
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) stop eidos-api eidos-trading eidos-matching eidos-market eidos-chain eidos-risk eidos-jobs eidos-admin
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) rm -f eidos-api eidos-trading eidos-matching eidos-market eidos-chain eidos-risk eidos-jobs eidos-admin
	@echo "Services stopped."

services-logs:
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) logs -f eidos-api eidos-trading eidos-matching eidos-market eidos-chain eidos-risk eidos-jobs eidos-admin

services-logs-%:
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) logs -f eidos-$*

services-restart:
	@echo "Restarting all application services..."
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) restart eidos-api eidos-trading eidos-matching eidos-market eidos-chain eidos-risk eidos-jobs eidos-admin
	@echo "Services restarted."

services-restart-%:
	@echo "Restarting eidos-$*..."
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) restart eidos-$*
	@echo "eidos-$* restarted."

services-status:
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) ps

# Start specific services
services-core:
	@echo "Starting core trading services (trading + matching)..."
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) up -d eidos-trading eidos-matching
	@echo "Core services started."

services-all:
	@echo "Starting all services including API..."
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) up -d
	@echo "All services started."

# ========================================
# Development Environment
# ========================================
dev-up: infra-up
	@echo ""
	@echo "Development infrastructure is ready!"
	@echo "Run 'make run-<service>' to start a service locally"
	@echo "Or run 'make services-up' to start all services in Docker"

dev-down:
	@echo "Stopping all containers..."
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) down
	@echo "All containers stopped."

dev-clean: infra-clean
	@echo "Development environment cleaned!"

# Local development with infrastructure
dev-local: infra-up
	@echo ""
	@echo "Infrastructure ready for local development!"
	@echo "Services can connect using:"
	@echo "  DB_HOST=localhost DB_PORT=5432"
	@echo "  REDIS_ADDR=localhost:6379"
	@echo "  KAFKA_BROKERS=localhost:29092"
	@echo "  NACOS_SERVER_ADDR=localhost:8848"

# ========================================
# Health Checks & Verification
# ========================================
health-check:
	@echo "Running health check..."
	@./scripts/health_check.sh

verify-data-flow:
	@echo "Verifying data flow..."
	@./scripts/verify_data_flow.sh

# ========================================
# Single Service Run (Local Development)
# ========================================
run-%:
	@echo "Running $*..."
	@cd $* && $(GOCMD) run ./cmd/main.go

# ========================================
# Docker
# ========================================
docker-build: $(addprefix docker-build-,$(SERVICES))

docker-build-%:
	@echo "Building Docker image for $*..."
	docker build -t eidos/$*:latest -f $*/Dockerfile .

docker-push: $(addprefix docker-push-,$(SERVICES))

docker-push-%:
	@echo "Pushing Docker image for $*..."
	docker push eidos/$*:latest

docker-tag:
	@echo "Tagging Docker images with version $(VERSION)..."
	@for svc in $(SERVICES); do \
		docker tag eidos/$$svc:latest eidos/$$svc:$(VERSION); \
	done

# ========================================
# Clean
# ========================================
clean:
	@for svc in $(SERVICES); do \
		echo "Cleaning $$svc..."; \
		rm -rf $$svc/bin; \
		rm -f $$svc/coverage.out $$svc/coverage.html; \
	done
	@find proto -name "*.pb.go" -delete 2>/dev/null || true
	@rm -rf tests/integration/coverage.out

clean-all: clean infra-clean
	@echo "All artifacts cleaned!"

# ========================================
# Dependency Management
# ========================================
mod-tidy:
	@for svc in $(SERVICES) eidos-common proto; do \
		if [ -d $$svc ]; then \
			echo "Tidying $$svc..."; \
			cd $$svc && $(GOMOD) tidy && cd ..; \
		fi \
	done
	@cd tests/integration && $(GOMOD) tidy

mod-download:
	@for svc in $(SERVICES) eidos-common proto; do \
		if [ -d $$svc ]; then \
			echo "Downloading deps for $$svc..."; \
			cd $$svc && $(GOMOD) download && cd ..; \
		fi \
	done

mod-verify:
	@for svc in $(SERVICES) eidos-common proto; do \
		if [ -d $$svc ]; then \
			echo "Verifying deps for $$svc..."; \
			cd $$svc && $(GOMOD) verify && cd ..; \
		fi \
	done

# ========================================
# Tool Installation
# ========================================
install-tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/grpc-ecosystem/grpc-health-probe@latest
	@echo "Tools installed successfully!"

# ========================================
# Database
# ========================================

# Database connection defaults (can be overridden via environment)
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_USER ?= eidos
DB_PASSWORD ?= eidos123
DB_NAME ?= eidos
MARKET_DB_PORT ?= 5433
MARKET_DB_NAME ?= eidos_market

# Seed database with test data
db-seed:
	@echo "Seeding trading database..."
	@PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) -f scripts/db/seed.sql
	@echo "Seeding market database..."
	@PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(MARKET_DB_PORT) -U $(DB_USER) -d $(MARKET_DB_NAME) -f scripts/db/seed_market.sql
	@echo "Seed data loaded!"

# Reset databases (CAUTION: destroys all data!)
# 服务启动时会自动执行迁移
db-reset:
	@echo "⚠️  WARNING: This will destroy ALL data!"
	@echo "Press Ctrl+C to cancel, or wait 3 seconds..."
	@sleep 3
	@echo "Resetting databases..."
	@PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d postgres -c "DROP DATABASE IF EXISTS $(DB_NAME);" -c "CREATE DATABASE $(DB_NAME);"
	@PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(MARKET_DB_PORT) -U $(DB_USER) -d postgres -c "DROP DATABASE IF EXISTS $(MARKET_DB_NAME);" -c "CREATE DATABASE $(MARKET_DB_NAME);"
	@echo "Database reset complete! Start services to run migrations."

# Create databases if they don't exist
db-create:
	@echo "Creating databases if they don't exist..."
	@PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d postgres -c "SELECT 'CREATE DATABASE $(DB_NAME)' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$(DB_NAME)')\gexec" 2>/dev/null || true
	@PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(MARKET_DB_PORT) -U $(DB_USER) -d postgres -c "SELECT 'CREATE DATABASE $(MARKET_DB_NAME)' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$(MARKET_DB_NAME)')\gexec" 2>/dev/null || true
	@echo "Databases ready!"

# Quick setup: create DBs, seed (migrations run on service startup)
db-setup: db-create db-seed
	@echo "Database setup complete! Start services to run migrations."

# ========================================
# Quick Commands
# ========================================
up: infra-up services-up
	@echo "Eidos Trading System is up and running!"

down: services-down infra-down
	@echo "Eidos Trading System stopped."

restart: down up
	@echo "Eidos Trading System restarted."

logs:
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) logs -f

ps:
	docker-compose -f $(DC_INFRA) -f $(DC_SERVICES) ps

# Legacy commands (for backwards compatibility with docker-compose.yml)
legacy-up:
	@echo "Using legacy docker-compose.yml..."
	docker-compose -f $(DC_LEGACY) up -d postgres timescaledb redis kafka kafka-init nacos prometheus grafana
	@sleep 15
	docker-compose -f $(DC_LEGACY) --profile services up -d

legacy-down:
	docker-compose -f $(DC_LEGACY) --profile services down
	docker-compose -f $(DC_LEGACY) down

# ========================================
# CI/CD
# ========================================
ci-test: lint vet test
	@echo "CI tests passed!"

ci-build: proto build docker-build
	@echo "CI build complete!"
