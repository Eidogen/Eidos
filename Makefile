.PHONY: all build test clean proto infra-up infra-down help

# 服务列表
SERVICES := eidos-api eidos-trading eidos-matching eidos-market eidos-chain eidos-risk eidos-jobs eidos-admin

# Go 参数
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOFMT := $(GOCMD) fmt
GOMOD := $(GOCMD) mod

# Proto 参数
PROTOC := protoc
PROTO_DIR := proto
PROTO_GO_OUT := --go_out=. --go_opt=paths=source_relative
PROTO_GRPC_OUT := --go-grpc_out=. --go-grpc_opt=paths=source_relative

# 默认目标
all: proto build

# ========================================
# 帮助
# ========================================
help:
	@echo "Eidos 交易系统 - Makefile 使用指南"
	@echo ""
	@echo "基础命令:"
	@echo "  make build          构建所有服务"
	@echo "  make test           运行所有测试"
	@echo "  make clean          清理构建产物"
	@echo "  make fmt            格式化代码"
	@echo "  make lint           运行代码检查"
	@echo ""
	@echo "Proto 相关:"
	@echo "  make proto          生成所有 proto 文件"
	@echo "  make proto-trading  生成 trading proto"
	@echo "  make proto-matching 生成 matching proto"
	@echo ""
	@echo "基础设施:"
	@echo "  make infra-up       启动基础设施 (PostgreSQL, Redis, Kafka, etc.)"
	@echo "  make infra-down     停止基础设施 (数据保留)"
	@echo "  make infra-clean    停止基础设施并删除所有数据 (慎用!)"
	@echo "  make infra-logs     查看基础设施日志"
	@echo "  make infra-status   查看基础设施状态"
	@echo ""
	@echo "单服务操作:"
	@echo "  make build-api      构建 eidos-api"
	@echo "  make run-api        运行 eidos-api"
	@echo "  make test-api       测试 eidos-api"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build   构建所有 Docker 镜像"
	@echo "  make docker-push    推送所有 Docker 镜像"

# ========================================
# 构建
# ========================================
build: $(addprefix build-,$(SERVICES))

build-%:
	@echo "Building $*..."
	@cd $* && $(GOBUILD) -o bin/$* ./cmd/main.go

# ========================================
# 测试
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

# ========================================
# 代码质量
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

# ========================================
# Proto 生成
# ========================================
# Proto 文件和生成的 Go 文件统一放在 proto 文件夹下
# 使用 module 模式，输出路径由 go_package 决定

proto: proto-clean proto-common proto-trading proto-matching proto-market proto-chain proto-risk proto-jobs proto-admin
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

# ========================================
# 基础设施
# ========================================
infra-up:
	@echo "Starting infrastructure..."
	docker-compose up -d postgres timescaledb redis kafka nacos prometheus grafana
	@echo "Waiting for services to be ready..."
	@sleep 10
	@echo "Infrastructure is ready!"
	@echo "  PostgreSQL:   localhost:5432"
	@echo "  TimescaleDB:  localhost:5433"
	@echo "  Redis:        localhost:6379"
	@echo "  Kafka:        localhost:29092"
	@echo "  Nacos:        http://localhost:8848/nacos"
	@echo "  Prometheus:   http://localhost:9090"
	@echo "  Grafana:      http://localhost:3000 (admin/admin123)"

infra-down:
	@echo "Stopping infrastructure (data preserved in volumes)..."
	docker-compose down
	@echo "Infrastructure stopped. Data is preserved in Docker volumes."

infra-logs:
	docker-compose logs -f

infra-clean:
	@echo "WARNING: This will delete all data in Docker volumes!"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	docker-compose down -v
	@echo "Infrastructure stopped and all data deleted."

infra-status:
	docker-compose ps

# ========================================
# 单服务运行
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
	@cd $* && docker build -t $*:latest -f Dockerfile ..

docker-push: $(addprefix docker-push-,$(SERVICES))

docker-push-%:
	@echo "Pushing Docker image for $*..."
	docker push $*:latest

# ========================================
# 清理
# ========================================
clean:
	@for svc in $(SERVICES); do \
		echo "Cleaning $$svc..."; \
		rm -rf $$svc/bin; \
		rm -f $$svc/coverage.out $$svc/coverage.html; \
	done
	@find proto -name "*.pb.go" -delete 2>/dev/null || true

# ========================================
# 依赖管理
# ========================================
mod-tidy:
	@for svc in $(SERVICES) eidos-common; do \
		echo "Tidying $$svc..."; \
		cd $$svc && $(GOMOD) tidy && cd ..; \
	done

mod-download:
	@for svc in $(SERVICES) eidos-common; do \
		echo "Downloading deps for $$svc..."; \
		cd $$svc && $(GOMOD) download && cd ..; \
	done

# ========================================
# 工具安装
# ========================================
install-tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installed successfully!"
