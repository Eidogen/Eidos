#!/bin/bash
#
# Eidos Services Stop Script
# Stops all microservices gracefully
#
# Usage: ./scripts/stop_all.sh [options]
#
# Options:
#   --service <name>  Stop only a specific service
#   --force           Force kill services (SIGKILL instead of SIGTERM)
#   --include-infra   Also stop infrastructure services (Docker)
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
SINGLE_SERVICE=""
FORCE=false
INCLUDE_INFRA=false
LOG_DIR="$PROJECT_ROOT/logs"

# Service list (reverse order for shutdown)
SERVICES=(
    "eidos-api"
    "eidos-admin"
    "eidos-jobs"
    "eidos-market"
    "eidos-matching"
    "eidos-trading"
    "eidos-chain"
    "eidos-risk"
)

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --service)
            SINGLE_SERVICE="$2"
            shift 2
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --include-infra)
            INCLUDE_INFRA=true
            shift
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --service <name>  Stop only a specific service"
            echo "  --force           Force kill services (SIGKILL)"
            echo "  --include-infra   Also stop infrastructure services"
            echo "  --help            Show this help message"
            echo ""
            echo "Services: ${SERVICES[*]}"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Eidos Services Stop Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Stop a service by its PID file
stop_service_by_pid() {
    local service=$1
    local pid_file="$LOG_DIR/$service.pid"

    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")

        if ps -p $pid > /dev/null 2>&1; then
            echo -e "${YELLOW}Stopping $service (PID: $pid)...${NC}"

            if [ "$FORCE" = true ]; then
                kill -9 $pid 2>/dev/null || true
            else
                kill -15 $pid 2>/dev/null || true

                # Wait for graceful shutdown (max 10 seconds)
                local count=0
                while ps -p $pid > /dev/null 2>&1 && [ $count -lt 10 ]; do
                    sleep 1
                    count=$((count + 1))
                done

                # Force kill if still running
                if ps -p $pid > /dev/null 2>&1; then
                    echo -e "${YELLOW}  Force killing $service...${NC}"
                    kill -9 $pid 2>/dev/null || true
                fi
            fi

            rm -f "$pid_file"
            echo -e "${GREEN}  Stopped${NC}"
            return 0
        else
            echo -e "${YELLOW}$service is not running (stale PID file)${NC}"
            rm -f "$pid_file"
            return 0
        fi
    fi

    return 1
}

# Stop a service by process name
stop_service_by_name() {
    local service=$1

    # Find process by name
    local pids=$(pgrep -f "bin/$service" 2>/dev/null || true)

    if [ -n "$pids" ]; then
        echo -e "${YELLOW}Stopping $service (PIDs: $pids)...${NC}"

        for pid in $pids; do
            if [ "$FORCE" = true ]; then
                kill -9 $pid 2>/dev/null || true
            else
                kill -15 $pid 2>/dev/null || true
            fi
        done

        # Wait for processes to stop
        sleep 2

        # Force kill any remaining
        pids=$(pgrep -f "bin/$service" 2>/dev/null || true)
        if [ -n "$pids" ]; then
            echo -e "${YELLOW}  Force killing remaining processes...${NC}"
            for pid in $pids; do
                kill -9 $pid 2>/dev/null || true
            done
        fi

        echo -e "${GREEN}  Stopped${NC}"
        return 0
    fi

    return 1
}

# Stop a single service
stop_service() {
    local service=$1

    # Try PID file first
    if stop_service_by_pid "$service"; then
        return 0
    fi

    # Try by process name
    if stop_service_by_name "$service"; then
        return 0
    fi

    echo -e "${BLUE}$service is not running${NC}"
    return 0
}

# Stop infrastructure services
stop_infrastructure() {
    if [ "$INCLUDE_INFRA" = false ]; then
        return 0
    fi

    echo -e "${YELLOW}Stopping infrastructure services...${NC}"

    cd "$PROJECT_ROOT"

    if [ -f "docker-compose.yml" ]; then
        docker-compose down
        echo -e "${GREEN}Infrastructure services stopped${NC}"
    else
        echo -e "${YELLOW}No docker-compose.yml found${NC}"
    fi
}

# Show service status
show_status() {
    echo ""
    echo -e "${BLUE}Service Status:${NC}"

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

    for service in "${SERVICES[@]}"; do
        local port=${SERVICE_PORTS[$service]}
        if nc -z localhost $port 2>/dev/null; then
            echo -e "  ${GREEN}[RUNNING]${NC} $service (port $port)"
        else
            echo -e "  ${RED}[STOPPED]${NC} $service (port $port)"
        fi
    done
}

# Main stop process
main() {
    cd "$PROJECT_ROOT"

    # If single service specified, stop only that service
    if [ -n "$SINGLE_SERVICE" ]; then
        if [[ ! " ${SERVICES[@]} " =~ " ${SINGLE_SERVICE} " ]]; then
            echo -e "${RED}Unknown service: $SINGLE_SERVICE${NC}"
            echo "Available services: ${SERVICES[*]}"
            exit 1
        fi

        stop_service "$SINGLE_SERVICE"
        show_status
        exit $?
    fi

    # Stop all services in reverse order
    echo -e "${YELLOW}Stopping all services...${NC}"
    echo ""

    for service in "${SERVICES[@]}"; do
        stop_service "$service"
    done

    # Stop infrastructure if requested
    stop_infrastructure

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  All services stopped${NC}"
    echo -e "${GREEN}========================================${NC}"

    show_status
}

# Run main function
main "$@"
