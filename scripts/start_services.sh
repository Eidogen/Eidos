#!/bin/bash
#
# Eidos Trading System - Start All Services Script
# Starts infrastructure and services in the correct order
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Configuration
DOCKER_COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"
TIMEOUT=${TIMEOUT:-300}  # 5 minutes default timeout
POLL_INTERVAL=${POLL_INTERVAL:-5}

# Infrastructure services
INFRA_SERVICES=(
    "postgres"
    "timescaledb"
    "redis"
    "kafka"
    "nacos"
)

# Application services (in startup order)
APP_SERVICES=(
    "eidos-trading"
    "eidos-matching"
    "eidos-market"
    "eidos-chain"
    "eidos-risk"
    "eidos-jobs"
    "eidos-admin"
    "eidos-api"
)

# Service ports for health checks
declare -A SERVICE_PORTS=(
    ["postgres"]="5432"
    ["timescaledb"]="5433"
    ["redis"]="6379"
    ["kafka"]="29092"
    ["nacos"]="8848"
    ["eidos-trading"]="50051"
    ["eidos-matching"]="50052"
    ["eidos-market"]="50053"
    ["eidos-chain"]="50054"
    ["eidos-risk"]="50055"
    ["eidos-jobs"]="50056"
    ["eidos-admin"]="8088"
    ["eidos-api"]="8080"
)

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

print_header() {
    echo ""
    echo "=============================================="
    echo "  Eidos Trading System - Service Starter"
    echo "=============================================="
    echo ""
}

# Check if Docker and docker-compose are available
check_prerequisites() {
    log_step "Checking prerequisites..."

    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi

    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi

    if [ ! -f "$DOCKER_COMPOSE_FILE" ]; then
        log_error "docker-compose.yml not found at: $DOCKER_COMPOSE_FILE"
        exit 1
    fi

    log_success "Prerequisites check passed"
}

# Check if a TCP port is open
check_port() {
    local host=$1
    local port=$2
    local timeout=${3:-5}

    nc -z -w "$timeout" "$host" "$port" 2>/dev/null
}

# Check if a HTTP endpoint is healthy
check_http_health() {
    local url=$1
    local timeout=${2:-5}

    curl -sf --connect-timeout "$timeout" "$url" > /dev/null 2>&1
}

# Wait for a service to be ready
wait_for_service() {
    local service=$1
    local port=${SERVICE_PORTS[$service]}
    local start_time=$(date +%s)
    local elapsed=0

    echo -n "    Waiting for $service (port $port)... "

    while [ $elapsed -lt $TIMEOUT ]; do
        # Check based on service type
        case $service in
            postgres|timescaledb)
                if PGPASSWORD="eidos123" psql -h localhost -p "$port" -U eidos -d postgres -c "SELECT 1" > /dev/null 2>&1; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
            redis)
                if redis-cli -h localhost -p "$port" PING 2>/dev/null | grep -q "PONG"; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
            kafka)
                if check_port "localhost" "$port"; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
            nacos)
                if check_http_health "http://localhost:$port/nacos/v1/console/health/readiness"; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
            eidos-api)
                if check_http_health "http://localhost:$port/health"; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
            eidos-admin)
                if check_port "localhost" "$port"; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
            *)
                # gRPC services - just check port
                if check_port "localhost" "$port"; then
                    echo -e "${GREEN}ready${NC} (${elapsed}s)"
                    return 0
                fi
                ;;
        esac

        sleep $POLL_INTERVAL
        elapsed=$(($(date +%s) - start_time))
    done

    echo -e "${RED}timeout${NC}"
    return 1
}

# Check if infrastructure is running
check_infrastructure() {
    log_step "Checking infrastructure status..."

    local all_running=true
    local running_services=()
    local missing_services=()

    for service in "${INFRA_SERVICES[@]}"; do
        local port=${SERVICE_PORTS[$service]}
        if check_port "localhost" "$port"; then
            running_services+=("$service")
        else
            missing_services+=("$service")
            all_running=false
        fi
    done

    if [ ${#running_services[@]} -gt 0 ]; then
        log_info "Running: ${running_services[*]}"
    fi

    if [ ${#missing_services[@]} -gt 0 ]; then
        log_warn "Not running: ${missing_services[*]}"
    fi

    echo ""

    if [ "$all_running" = true ]; then
        return 0
    else
        return 1
    fi
}

# Start infrastructure services
start_infrastructure() {
    log_step "Starting infrastructure services..."

    cd "$PROJECT_ROOT"

    # Start infrastructure containers
    docker compose up -d postgres timescaledb redis kafka kafka-init nacos prometheus grafana

    echo ""
    log_info "Waiting for infrastructure to be ready..."
    echo ""

    local failed=false

    for service in "${INFRA_SERVICES[@]}"; do
        if ! wait_for_service "$service"; then
            log_error "Failed to start $service"
            failed=true
        fi
    done

    echo ""

    if [ "$failed" = true ]; then
        log_error "Some infrastructure services failed to start"
        return 1
    fi

    log_success "Infrastructure is ready"
    return 0
}

# Start application services
start_application_services() {
    log_step "Starting application services..."

    cd "$PROJECT_ROOT"

    # Start application containers
    docker compose --profile services up -d

    echo ""
    log_info "Waiting for services to be ready..."
    echo ""

    local failed=false

    for service in "${APP_SERVICES[@]}"; do
        if ! wait_for_service "$service"; then
            log_error "Failed to start $service"
            failed=true
        fi
    done

    echo ""

    if [ "$failed" = true ]; then
        log_error "Some application services failed to start"
        return 1
    fi

    log_success "All application services are ready"
    return 0
}

# Print service status
print_service_status() {
    echo ""
    echo "=============================================="
    echo "  Service Status"
    echo "=============================================="
    echo ""

    log_info "Infrastructure Services:"
    for service in "${INFRA_SERVICES[@]}"; do
        local port=${SERVICE_PORTS[$service]}
        if check_port "localhost" "$port"; then
            echo -e "    ${GREEN}[OK]${NC} $service (port $port)"
        else
            echo -e "    ${RED}[X]${NC}  $service (port $port)"
        fi
    done

    echo ""
    log_info "Application Services:"
    for service in "${APP_SERVICES[@]}"; do
        local port=${SERVICE_PORTS[$service]}
        if check_port "localhost" "$port"; then
            echo -e "    ${GREEN}[OK]${NC} $service (port $port)"
        else
            echo -e "    ${RED}[X]${NC}  $service (port $port)"
        fi
    done

    echo ""
}

# Print connection info
print_connection_info() {
    echo ""
    echo "=============================================="
    echo "  Service Endpoints"
    echo "=============================================="
    echo ""
    echo "  Infrastructure:"
    echo "    PostgreSQL:   localhost:5432 (user: eidos, pass: eidos123)"
    echo "    TimescaleDB:  localhost:5433 (user: eidos, pass: eidos123)"
    echo "    Redis:        localhost:6379"
    echo "    Kafka:        localhost:29092"
    echo "    Nacos:        http://localhost:8848/nacos"
    echo ""
    echo "  Application Services:"
    echo "    API Gateway:     http://localhost:8080"
    echo "    Admin Portal:    http://localhost:8088"
    echo "    Trading (gRPC):  localhost:50051"
    echo "    Matching (gRPC): localhost:50052"
    echo "    Market (gRPC):   localhost:50053"
    echo "    Chain (gRPC):    localhost:50054"
    echo "    Risk (gRPC):     localhost:50055"
    echo "    Jobs (gRPC):     localhost:50056"
    echo ""
    echo "  Monitoring:"
    echo "    Prometheus:   http://localhost:9090"
    echo "    Grafana:      http://localhost:3000 (admin/admin123)"
    echo ""
}

# Stop all services
stop_all_services() {
    log_step "Stopping all services..."

    cd "$PROJECT_ROOT"

    docker compose --profile services --profile debug down

    log_success "All services stopped"
}

# Restart all services
restart_all_services() {
    log_step "Restarting all services..."

    stop_all_services

    sleep 5

    start_infrastructure
    start_application_services

    print_service_status
    print_connection_info
}

# Show logs
show_logs() {
    local service=$1

    cd "$PROJECT_ROOT"

    if [ -n "$service" ]; then
        docker compose logs -f "$service"
    else
        docker compose --profile services logs -f
    fi
}

# Show help
show_help() {
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo ""
    echo "Commands:"
    echo "  start         Start all services (default)"
    echo "  stop          Stop all services"
    echo "  restart       Restart all services"
    echo "  status        Show service status"
    echo "  infra         Start infrastructure only"
    echo "  services      Start application services only (requires infra)"
    echo "  logs [svc]    Show service logs (all or specific service)"
    echo "  help          Show this help message"
    echo ""
    echo "Options:"
    echo "  --timeout N   Set timeout in seconds (default: 300)"
    echo "  --poll N      Set poll interval in seconds (default: 5)"
    echo ""
    echo "Examples:"
    echo "  $0                   # Start all services"
    echo "  $0 start             # Same as above"
    echo "  $0 infra             # Start infrastructure only"
    echo "  $0 services          # Start app services (infra must be running)"
    echo "  $0 status            # Show current status"
    echo "  $0 stop              # Stop everything"
    echo "  $0 logs eidos-api    # Show eidos-api logs"
}

# Main execution
main() {
    local command="start"

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            start|stop|restart|status|infra|services|logs|help)
                command=$1
                shift
                ;;
            --timeout)
                TIMEOUT=$2
                shift 2
                ;;
            --poll)
                POLL_INTERVAL=$2
                shift 2
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                # For logs command, this might be the service name
                if [ "$command" = "logs" ]; then
                    show_logs "$1"
                    exit 0
                fi
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done

    print_header
    check_prerequisites

    case $command in
        start)
            if ! check_infrastructure; then
                start_infrastructure
            else
                log_info "Infrastructure is already running"
            fi
            echo ""
            start_application_services
            print_service_status
            print_connection_info
            ;;
        stop)
            stop_all_services
            ;;
        restart)
            restart_all_services
            ;;
        status)
            print_service_status
            ;;
        infra)
            start_infrastructure
            print_service_status
            ;;
        services)
            if ! check_infrastructure; then
                log_error "Infrastructure is not running. Start it first with: $0 infra"
                exit 1
            fi
            start_application_services
            print_service_status
            ;;
        logs)
            show_logs ""
            ;;
        help)
            show_help
            ;;
    esac
}

# Run main
main "$@"
