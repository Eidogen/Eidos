#!/bin/bash
#
# Eidos Services Startup Script
# Starts all microservices in the correct order with proper dependencies
#
# Usage: ./scripts/start_all.sh [options]
#
# Options:
#   --env <env>       Environment (dev, staging, prod) - default: dev
#   --skip-infra      Skip infrastructure services (assumes already running)
#   --service <name>  Start only a specific service
#   --detach          Run services in background (detached mode)
#   --help            Show this help message
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get the project root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Default values
ENV="dev"
SKIP_INFRA=false
SINGLE_SERVICE=""
DETACH=false
LOG_DIR="$PROJECT_ROOT/logs"

# Service definitions with their ports and dependencies
declare -A SERVICE_PORTS=(
    ["eidos-trading"]="50051"
    ["eidos-matching"]="50052"
    ["eidos-market"]="50053"
    ["eidos-chain"]="50054"
    ["eidos-risk"]="50055"
    ["eidos-jobs"]="50056"
    ["eidos-admin"]="8088"
    ["eidos-api"]="8080"
)

# Service startup order (based on dependencies)
SERVICE_ORDER=(
    "eidos-risk"      # Risk service - no dependencies on other services
    "eidos-chain"     # Chain service - blockchain integration
    "eidos-trading"   # Trading service - core order management
    "eidos-matching"  # Matching engine - depends on trading
    "eidos-market"    # Market data service
    "eidos-jobs"      # Background jobs
    "eidos-admin"     # Admin service
    "eidos-api"       # API Gateway - depends on all services
)

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --env)
            ENV="$2"
            shift 2
            ;;
        --skip-infra)
            SKIP_INFRA=true
            shift
            ;;
        --service)
            SINGLE_SERVICE="$2"
            shift 2
            ;;
        --detach)
            DETACH=true
            shift
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --env <env>       Environment (dev, staging, prod) - default: dev"
            echo "  --skip-infra      Skip infrastructure services"
            echo "  --service <name>  Start only a specific service"
            echo "  --detach          Run services in background"
            echo "  --help            Show this help message"
            echo ""
            echo "Services: ${SERVICE_ORDER[*]}"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Eidos Services Startup Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${BLUE}Environment: ${ENV}${NC}"
echo ""

# Create log directory
mkdir -p "$LOG_DIR"

# Load environment variables
load_env() {
    local env_file="$PROJECT_ROOT/.env"
    local env_specific="$PROJECT_ROOT/.env.$ENV"

    if [ -f "$env_file" ]; then
        echo -e "${YELLOW}Loading environment from .env${NC}"
        set -a
        source "$env_file"
        set +a
    fi

    if [ -f "$env_specific" ]; then
        echo -e "${YELLOW}Loading environment from .env.$ENV${NC}"
        set -a
        source "$env_specific"
        set +a
    fi
}

# Check if a port is available
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 1
    fi
    return 0
}

# Wait for a service to be ready
wait_for_service() {
    local service=$1
    local port=$2
    local max_attempts=30
    local attempt=1

    echo -e "${YELLOW}Waiting for $service to be ready on port $port...${NC}"

    while [ $attempt -le $max_attempts ]; do
        if nc -z localhost $port 2>/dev/null; then
            echo -e "${GREEN}$service is ready!${NC}"
            return 0
        fi
        sleep 1
        attempt=$((attempt + 1))
    done

    echo -e "${RED}$service failed to start within $max_attempts seconds${NC}"
    return 1
}

# Start infrastructure services using docker-compose
start_infrastructure() {
    if [ "$SKIP_INFRA" = true ]; then
        echo -e "${YELLOW}Skipping infrastructure services${NC}"
        return 0
    fi

    echo -e "${GREEN}[Infrastructure] Starting infrastructure services...${NC}"

    cd "$PROJECT_ROOT"

    # Start infrastructure services
    docker-compose up -d postgres redis kafka zookeeper nacos

    echo -e "${YELLOW}Waiting for infrastructure to be ready...${NC}"

    # Wait for PostgreSQL
    echo -n "Waiting for PostgreSQL..."
    until docker-compose exec -T postgres pg_isready -U eidos >/dev/null 2>&1; do
        echo -n "."
        sleep 1
    done
    echo -e " ${GREEN}Ready${NC}"

    # Wait for Redis
    echo -n "Waiting for Redis..."
    until docker-compose exec -T redis redis-cli ping >/dev/null 2>&1; do
        echo -n "."
        sleep 1
    done
    echo -e " ${GREEN}Ready${NC}"

    # Wait for Kafka
    echo -n "Waiting for Kafka..."
    sleep 5  # Kafka takes longer to initialize
    until docker-compose exec -T kafka kafka-topics.sh --list --bootstrap-server localhost:9092 >/dev/null 2>&1; do
        echo -n "."
        sleep 2
    done
    echo -e " ${GREEN}Ready${NC}"

    echo -e "${GREEN}Infrastructure services are ready!${NC}"
    echo ""
}

# Start a single service
start_service() {
    local service=$1
    local port=${SERVICE_PORTS[$service]}
    local service_dir="$PROJECT_ROOT/$service"

    # Check if service directory exists
    if [ ! -d "$service_dir" ]; then
        echo -e "${RED}Service directory not found: $service_dir${NC}"
        return 1
    fi

    # Check if port is available
    if ! check_port $port; then
        echo -e "${YELLOW}Port $port is already in use (${service} may already be running)${NC}"
        return 0
    fi

    echo -e "${GREEN}Starting ${service} on port ${port}...${NC}"

    cd "$service_dir"

    # Build the service
    echo -e "${YELLOW}  Building...${NC}"
    go build -o "$service_dir/bin/$service" "$service_dir/cmd/main.go" 2>&1 || {
        echo -e "${RED}  Failed to build $service${NC}"
        return 1
    }

    # Set environment variables
    export SERVICE_ENV=$ENV
    export CONFIG_PATH="$service_dir/config/config.yaml"

    # Start the service
    if [ "$DETACH" = true ]; then
        echo -e "${YELLOW}  Starting in background...${NC}"
        nohup "$service_dir/bin/$service" > "$LOG_DIR/$service.log" 2>&1 &
        echo $! > "$LOG_DIR/$service.pid"
        echo -e "${GREEN}  Started (PID: $(cat $LOG_DIR/$service.pid))${NC}"
    else
        echo -e "${YELLOW}  Starting in foreground (Ctrl+C to stop)...${NC}"
        "$service_dir/bin/$service"
    fi

    # Wait for service to be ready (only in detached mode)
    if [ "$DETACH" = true ]; then
        wait_for_service $service $port
    fi

    return 0
}

# Main startup process
main() {
    cd "$PROJECT_ROOT"

    # Load environment
    load_env

    # Start infrastructure if not skipped
    start_infrastructure

    # If single service specified, start only that service
    if [ -n "$SINGLE_SERVICE" ]; then
        if [[ ! " ${SERVICE_ORDER[@]} " =~ " ${SINGLE_SERVICE} " ]]; then
            echo -e "${RED}Unknown service: $SINGLE_SERVICE${NC}"
            echo "Available services: ${SERVICE_ORDER[*]}"
            exit 1
        fi

        start_service "$SINGLE_SERVICE"
        exit $?
    fi

    # Start all services in order
    echo -e "${GREEN}Starting all services...${NC}"
    echo ""

    for service in "${SERVICE_ORDER[@]}"; do
        start_service "$service"

        # Small delay between service starts
        if [ "$DETACH" = true ]; then
            sleep 2
        fi
    done

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  All services started successfully!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${BLUE}Service Status:${NC}"
    for service in "${SERVICE_ORDER[@]}"; do
        local port=${SERVICE_PORTS[$service]}
        if nc -z localhost $port 2>/dev/null; then
            echo -e "  ${GREEN}[RUNNING]${NC} $service (port $port)"
        else
            echo -e "  ${RED}[STOPPED]${NC} $service (port $port)"
        fi
    done
    echo ""
    echo -e "${YELLOW}Logs are available in: $LOG_DIR${NC}"
    echo -e "${YELLOW}To stop all services, run: ./scripts/stop_all.sh${NC}"
}

# Run main function
main "$@"
