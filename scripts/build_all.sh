#!/bin/bash
#
# Eidos Trading System - Build All Services Script
# Compiles all services using golang:1.22-alpine Docker image
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

# Build configuration
GO_IMAGE="golang:1.22-alpine"
OUTPUT_DIR="${PROJECT_ROOT}/bin"

# Services in dependency order
# 1. eidos-common (shared library)
# 2. proto (protobuf definitions)
# 3. Application services
SHARED_MODULES=(
    "eidos-common"
    "proto"
)

SERVICES=(
    "eidos-trading"
    "eidos-matching"
    "eidos-market"
    "eidos-chain"
    "eidos-risk"
    "eidos-jobs"
    "eidos-admin"
    "eidos-api"
)

# Counters
BUILD_SUCCESS=0
BUILD_FAILED=0
FAILED_SERVICES=()

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
    echo "  Eidos Trading System - Build All Services"
    echo "=============================================="
    echo ""
}

print_summary() {
    echo ""
    echo "=============================================="
    echo "  Build Summary"
    echo "=============================================="
    echo ""
    echo -e "  ${GREEN}Successful:${NC} $BUILD_SUCCESS"
    echo -e "  ${RED}Failed:${NC}     $BUILD_FAILED"
    echo ""

    if [ ${#FAILED_SERVICES[@]} -gt 0 ]; then
        echo -e "  ${RED}Failed services:${NC}"
        for svc in "${FAILED_SERVICES[@]}"; do
            echo "    - $svc"
        done
        echo ""
    fi

    if [ $BUILD_FAILED -eq 0 ]; then
        echo -e "${GREEN}All services built successfully!${NC}"
        echo ""
        echo "Build artifacts are located in:"
        echo "  $OUTPUT_DIR/"
        return 0
    else
        echo -e "${RED}Some services failed to build. Please check the output above.${NC}"
        return 1
    fi
}

# Check if Docker is available
check_docker() {
    log_step "Checking Docker availability..."
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi

    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi

    log_success "Docker is available"
}

# Pull Go image if not exists
pull_go_image() {
    log_step "Ensuring Go Docker image is available..."
    if ! docker image inspect "$GO_IMAGE" &> /dev/null; then
        log_info "Pulling $GO_IMAGE..."
        docker pull "$GO_IMAGE"
    fi
    log_success "Go Docker image ready: $GO_IMAGE"
}

# Create output directory
create_output_dir() {
    log_step "Creating output directory..."
    mkdir -p "$OUTPUT_DIR"
    log_success "Output directory: $OUTPUT_DIR"
}

# Build shared modules (download dependencies)
build_shared_modules() {
    log_step "Building shared modules..."
    echo ""

    for module in "${SHARED_MODULES[@]}"; do
        local module_path="${PROJECT_ROOT}/${module}"

        if [ ! -d "$module_path" ]; then
            log_warn "Module directory not found: $module"
            continue
        fi

        echo -n "  Building $module... "

        # Run go mod download in Docker
        if docker run --rm \
            -v "${PROJECT_ROOT}:/app" \
            -w "/app/${module}" \
            -e CGO_ENABLED=0 \
            -e GOOS=linux \
            -e GOARCH=amd64 \
            "$GO_IMAGE" \
            sh -c "apk add --no-cache git && go mod download && go build ./..." \
            > /dev/null 2>&1; then
            log_success "OK"
        else
            log_warn "Dependencies downloaded (build may have warnings)"
        fi
    done

    echo ""
}

# Build a single service
build_service() {
    local service=$1
    local service_path="${PROJECT_ROOT}/${service}"
    local output_path="${OUTPUT_DIR}/${service}"

    if [ ! -d "$service_path" ]; then
        log_error "Service directory not found: $service"
        ((BUILD_FAILED++))
        FAILED_SERVICES+=("$service")
        return 1
    fi

    # Check if cmd/main.go exists
    if [ ! -f "${service_path}/cmd/main.go" ]; then
        log_warn "No cmd/main.go found for $service, skipping..."
        return 0
    fi

    echo -n "  Building $service... "

    # Build command using Docker
    local build_output
    build_output=$(docker run --rm \
        -v "${PROJECT_ROOT}:/app" \
        -w "/app/${service}" \
        -e CGO_ENABLED=0 \
        -e GOOS=linux \
        -e GOARCH=amd64 \
        "$GO_IMAGE" \
        sh -c "
            apk add --no-cache git > /dev/null 2>&1
            go build -ldflags='-s -w' -o /app/bin/${service} ./cmd/main.go
        " 2>&1)

    local build_status=$?

    if [ $build_status -eq 0 ] && [ -f "$output_path" ]; then
        local file_size=$(du -h "$output_path" | cut -f1)
        log_success "OK ($file_size)"
        ((BUILD_SUCCESS++))
        return 0
    else
        log_error "FAILED"
        if [ -n "$build_output" ]; then
            echo "    Error details:"
            echo "$build_output" | sed 's/^/      /'
        fi
        ((BUILD_FAILED++))
        FAILED_SERVICES+=("$service")
        return 1
    fi
}

# Build all services
build_all_services() {
    log_step "Building application services..."
    echo ""

    for service in "${SERVICES[@]}"; do
        build_service "$service"
    done

    echo ""
}

# Alternative: Build using local Go (if Docker is not preferred)
build_local() {
    log_info "Building services locally (without Docker)..."

    for service in "${SERVICES[@]}"; do
        local service_path="${PROJECT_ROOT}/${service}"

        if [ ! -d "$service_path" ] || [ ! -f "${service_path}/cmd/main.go" ]; then
            continue
        fi

        echo -n "  Building $service... "

        cd "$service_path"
        if CGO_ENABLED=0 go build -ldflags='-s -w' -o "${OUTPUT_DIR}/${service}" ./cmd/main.go 2>/dev/null; then
            log_success "OK"
            ((BUILD_SUCCESS++))
        else
            log_error "FAILED"
            ((BUILD_FAILED++))
            FAILED_SERVICES+=("$service")
        fi
        cd "$PROJECT_ROOT"
    done
}

# Clean build artifacts
clean_build() {
    log_step "Cleaning build artifacts..."
    rm -rf "$OUTPUT_DIR"
    log_success "Build artifacts cleaned"
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --local       Build using local Go instead of Docker"
    echo "  --clean       Clean build artifacts before building"
    echo "  --clean-only  Only clean build artifacts (no build)"
    echo "  --service     Build specific service (e.g., --service eidos-api)"
    echo "  --help        Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                    # Build all services using Docker"
    echo "  $0 --local            # Build all services using local Go"
    echo "  $0 --clean            # Clean and rebuild all services"
    echo "  $0 --service eidos-api # Build only eidos-api"
}

# Main execution
main() {
    local use_local=false
    local do_clean=false
    local clean_only=false
    local single_service=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --local)
                use_local=true
                shift
                ;;
            --clean)
                do_clean=true
                shift
                ;;
            --clean-only)
                clean_only=true
                shift
                ;;
            --service)
                single_service="$2"
                shift 2
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done

    print_header

    # Clean only mode
    if [ "$clean_only" = true ]; then
        clean_build
        exit 0
    fi

    # Clean before build
    if [ "$do_clean" = true ]; then
        clean_build
    fi

    # Create output directory
    create_output_dir

    # Single service build
    if [ -n "$single_service" ]; then
        if [ "$use_local" = true ]; then
            log_info "Building $single_service locally..."
            cd "${PROJECT_ROOT}/${single_service}"
            CGO_ENABLED=0 go build -ldflags='-s -w' -o "${OUTPUT_DIR}/${single_service}" ./cmd/main.go
        else
            check_docker
            pull_go_image
            build_service "$single_service"
        fi
        print_summary
        exit $?
    fi

    # Full build
    if [ "$use_local" = true ]; then
        log_info "Building all services locally..."
        build_local
    else
        check_docker
        pull_go_image
        build_shared_modules
        build_all_services
    fi

    print_summary
    exit $?
}

# Run main
main "$@"
