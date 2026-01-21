#!/bin/bash
#
# Eidos Trading System - Run All Unit Tests Script
# Runs unit tests for all services using golang:1.22-alpine Docker image
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

# Test configuration
GO_IMAGE="golang:1.22-alpine"
COVERAGE_DIR="${PROJECT_ROOT}/coverage"
VERBOSE=${VERBOSE:-false}
RACE_DETECTION=${RACE_DETECTION:-false}
COVERAGE=${COVERAGE:-false}

# All modules to test (in dependency order)
ALL_MODULES=(
    "eidos-common"
    "proto"
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
TEST_PASSED=0
TEST_FAILED=0
TEST_SKIPPED=0
FAILED_MODULES=()

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
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
    echo "  Eidos Trading System - Unit Test Runner"
    echo "=============================================="
    echo ""
    echo "Configuration:"
    echo "  Verbose:        $VERBOSE"
    echo "  Race Detection: $RACE_DETECTION"
    echo "  Coverage:       $COVERAGE"
    echo ""
}

print_summary() {
    echo ""
    echo "=============================================="
    echo "  Test Summary"
    echo "=============================================="
    echo ""
    echo -e "  ${GREEN}Passed:${NC}  $TEST_PASSED modules"
    echo -e "  ${RED}Failed:${NC}  $TEST_FAILED modules"
    echo -e "  ${YELLOW}Skipped:${NC} $TEST_SKIPPED modules"
    echo ""

    if [ ${#FAILED_MODULES[@]} -gt 0 ]; then
        echo -e "  ${RED}Failed modules:${NC}"
        for module in "${FAILED_MODULES[@]}"; do
            echo "    - $module"
        done
        echo ""
    fi

    if [ "$COVERAGE" = true ] && [ -d "$COVERAGE_DIR" ]; then
        echo "Coverage reports available at:"
        echo "  $COVERAGE_DIR/"
        echo ""
    fi

    if [ $TEST_FAILED -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        return 0
    else
        echo -e "${RED}Some tests failed. Please check the output above.${NC}"
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

# Create coverage directory
create_coverage_dir() {
    if [ "$COVERAGE" = true ]; then
        log_step "Creating coverage directory..."
        mkdir -p "$COVERAGE_DIR"
        log_success "Coverage directory: $COVERAGE_DIR"
    fi
}

# Check if module has tests
has_tests() {
    local module_path=$1
    local test_count

    # Count test files
    test_count=$(find "$module_path" -name "*_test.go" 2>/dev/null | wc -l)

    [ "$test_count" -gt 0 ]
}

# Run tests for a single module
test_module() {
    local module=$1
    local module_path="${PROJECT_ROOT}/${module}"
    local start_time
    local end_time
    local duration

    if [ ! -d "$module_path" ]; then
        log_warn "$module: Directory not found, skipping"
        ((TEST_SKIPPED++))
        return 0
    fi

    # Check for go.mod
    if [ ! -f "${module_path}/go.mod" ]; then
        log_warn "$module: No go.mod found, skipping"
        ((TEST_SKIPPED++))
        return 0
    fi

    # Check for test files
    if ! has_tests "$module_path"; then
        log_warn "$module: No test files found, skipping"
        ((TEST_SKIPPED++))
        return 0
    fi

    echo -n "  Testing $module... "
    start_time=$(date +%s)

    # Build test command
    local test_cmd="go test"

    if [ "$VERBOSE" = true ]; then
        test_cmd="$test_cmd -v"
    fi

    if [ "$RACE_DETECTION" = true ]; then
        test_cmd="$test_cmd -race"
    fi

    if [ "$COVERAGE" = true ]; then
        test_cmd="$test_cmd -coverprofile=/app/coverage/${module}.out -covermode=atomic"
    fi

    test_cmd="$test_cmd ./..."

    # Run tests in Docker
    local test_output
    test_output=$(docker run --rm \
        -v "${PROJECT_ROOT}:/app" \
        -w "/app/${module}" \
        -e CGO_ENABLED=0 \
        "$GO_IMAGE" \
        sh -c "
            apk add --no-cache git > /dev/null 2>&1
            $test_cmd
        " 2>&1)

    local test_status=$?
    end_time=$(date +%s)
    duration=$((end_time - start_time))

    if [ $test_status -eq 0 ]; then
        # Count tests from output
        local test_count=$(echo "$test_output" | grep -c "^--- PASS\|^=== RUN" 2>/dev/null || echo "0")
        log_success "OK (${duration}s)"
        ((TEST_PASSED++))

        if [ "$VERBOSE" = true ]; then
            echo "$test_output" | sed 's/^/      /'
        fi

        return 0
    else
        log_error "FAILED (${duration}s)"
        ((TEST_FAILED++))
        FAILED_MODULES+=("$module")

        # Show error output
        echo "    Error details:"
        echo "$test_output" | tail -50 | sed 's/^/      /'
        echo ""

        return 1
    fi
}

# Run tests for all modules
test_all_modules() {
    log_step "Running unit tests for all modules..."
    echo ""

    for module in "${ALL_MODULES[@]}"; do
        test_module "$module"
    done

    echo ""
}

# Run tests locally (without Docker)
test_local() {
    log_step "Running tests locally (without Docker)..."
    echo ""

    for module in "${ALL_MODULES[@]}"; do
        local module_path="${PROJECT_ROOT}/${module}"

        if [ ! -d "$module_path" ] || [ ! -f "${module_path}/go.mod" ]; then
            continue
        fi

        if ! has_tests "$module_path"; then
            log_warn "$module: No test files found, skipping"
            ((TEST_SKIPPED++))
            continue
        fi

        echo -n "  Testing $module... "

        cd "$module_path"

        local test_cmd="go test"
        [ "$VERBOSE" = true ] && test_cmd="$test_cmd -v"
        [ "$RACE_DETECTION" = true ] && test_cmd="$test_cmd -race"
        [ "$COVERAGE" = true ] && test_cmd="$test_cmd -coverprofile=${COVERAGE_DIR}/${module}.out -covermode=atomic"
        test_cmd="$test_cmd ./..."

        if eval "$test_cmd" > /dev/null 2>&1; then
            log_success "OK"
            ((TEST_PASSED++))
        else
            log_error "FAILED"
            ((TEST_FAILED++))
            FAILED_MODULES+=("$module")
        fi

        cd "$PROJECT_ROOT"
    done

    echo ""
}

# Generate HTML coverage report
generate_coverage_html() {
    if [ "$COVERAGE" != true ]; then
        return
    fi

    log_step "Generating HTML coverage reports..."

    for coverage_file in "${COVERAGE_DIR}"/*.out; do
        if [ -f "$coverage_file" ]; then
            local module_name=$(basename "$coverage_file" .out)
            local html_file="${COVERAGE_DIR}/${module_name}.html"

            docker run --rm \
                -v "${PROJECT_ROOT}:/app" \
                -w "/app" \
                "$GO_IMAGE" \
                go tool cover -html="/app/coverage/${module_name}.out" -o "/app/coverage/${module_name}.html" \
                2>/dev/null || true
        fi
    done

    log_success "Coverage HTML reports generated"
}

# Merge coverage reports
merge_coverage() {
    if [ "$COVERAGE" != true ]; then
        return
    fi

    log_step "Merging coverage reports..."

    # Create combined coverage file
    local combined_file="${COVERAGE_DIR}/combined.out"
    echo "mode: atomic" > "$combined_file"

    for coverage_file in "${COVERAGE_DIR}"/*.out; do
        if [ -f "$coverage_file" ] && [ "$(basename "$coverage_file")" != "combined.out" ]; then
            grep -v "^mode:" "$coverage_file" >> "$combined_file" 2>/dev/null || true
        fi
    done

    log_success "Combined coverage: $combined_file"
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS] [MODULES...]"
    echo ""
    echo "Options:"
    echo "  --local       Run tests using local Go instead of Docker"
    echo "  --verbose     Show verbose test output"
    echo "  --race        Enable race condition detection"
    echo "  --coverage    Generate coverage reports"
    echo "  --html        Generate HTML coverage reports (implies --coverage)"
    echo "  --help        Show this help message"
    echo ""
    echo "Environment Variables:"
    echo "  VERBOSE=true          Enable verbose output"
    echo "  RACE_DETECTION=true   Enable race detection"
    echo "  COVERAGE=true         Enable coverage reports"
    echo ""
    echo "Examples:"
    echo "  $0                              # Run all tests"
    echo "  $0 --verbose                    # Run all tests with verbose output"
    echo "  $0 --coverage --html            # Run tests with HTML coverage"
    echo "  $0 --local --race               # Run locally with race detection"
    echo "  $0 eidos-trading eidos-api      # Run tests for specific modules"
}

# Main execution
main() {
    local use_local=false
    local generate_html=false
    local specific_modules=()

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --local)
                use_local=true
                shift
                ;;
            --verbose|-v)
                VERBOSE=true
                shift
                ;;
            --race)
                RACE_DETECTION=true
                shift
                ;;
            --coverage)
                COVERAGE=true
                shift
                ;;
            --html)
                COVERAGE=true
                generate_html=true
                shift
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            -*)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
            *)
                # Assume it's a module name
                specific_modules+=("$1")
                shift
                ;;
        esac
    done

    print_header

    # Create coverage directory if needed
    create_coverage_dir

    # If specific modules provided, use them
    if [ ${#specific_modules[@]} -gt 0 ]; then
        ALL_MODULES=("${specific_modules[@]}")
    fi

    # Run tests
    if [ "$use_local" = true ]; then
        test_local
    else
        check_docker
        pull_go_image
        test_all_modules
    fi

    # Generate coverage reports
    if [ "$COVERAGE" = true ]; then
        merge_coverage
        if [ "$generate_html" = true ]; then
            generate_coverage_html
        fi
    fi

    print_summary
    exit $?
}

# Run main
main "$@"
