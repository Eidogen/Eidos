#!/bin/bash
#
# Eidos Trading System - Health Check Script
# This script checks the health of all services and infrastructure components
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration (can be overridden by environment variables)
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-eidos}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-eidos123}"
POSTGRES_DB="${POSTGRES_DB:-eidos}"

TIMESCALEDB_HOST="${TIMESCALEDB_HOST:-localhost}"
TIMESCALEDB_PORT="${TIMESCALEDB_PORT:-5433}"

REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"

KAFKA_BROKER="${KAFKA_BROKER:-localhost:29092}"

NACOS_HOST="${NACOS_HOST:-localhost}"
NACOS_PORT="${NACOS_PORT:-8848}"

# Service ports
API_PORT="${API_PORT:-8080}"
TRADING_PORT="${TRADING_PORT:-50051}"
MATCHING_PORT="${MATCHING_PORT:-50052}"
MARKET_PORT="${MARKET_PORT:-50053}"
CHAIN_PORT="${CHAIN_PORT:-50054}"
RISK_PORT="${RISK_PORT:-50055}"
JOBS_PORT="${JOBS_PORT:-50056}"
ADMIN_PORT="${ADMIN_PORT:-8088}"

# Counters
PASSED=0
FAILED=0
WARNINGS=0

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED++))
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    ((WARNINGS++))
}

check_tcp_port() {
    local host=$1
    local port=$2
    local timeout=${3:-5}

    if nc -z -w "$timeout" "$host" "$port" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

check_http_endpoint() {
    local url=$1
    local expected_status=${2:-200}
    local timeout=${3:-5}

    status_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout "$timeout" "$url" 2>/dev/null || echo "000")

    if [ "$status_code" == "$expected_status" ]; then
        return 0
    else
        return 1
    fi
}

check_grpc_service() {
    local host=$1
    local port=$2

    # Simple TCP check for gRPC (gRPC uses HTTP/2 over TCP)
    if check_tcp_port "$host" "$port"; then
        return 0
    else
        return 1
    fi
}

# ============================================================================
# Infrastructure Health Checks
# ============================================================================

echo ""
echo "=============================================="
echo "  Eidos Trading System - Health Check"
echo "=============================================="
echo ""

log_info "Checking Infrastructure Components..."
echo ""

# PostgreSQL
echo -n "  PostgreSQL ($POSTGRES_HOST:$POSTGRES_PORT): "
if check_tcp_port "$POSTGRES_HOST" "$POSTGRES_PORT"; then
    # Try to connect and run a simple query
    if PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT 1" >/dev/null 2>&1; then
        log_success "OK"
    else
        log_warn "Port open but query failed"
    fi
else
    log_fail "Not reachable"
fi

# TimescaleDB
echo -n "  TimescaleDB ($TIMESCALEDB_HOST:$TIMESCALEDB_PORT): "
if check_tcp_port "$TIMESCALEDB_HOST" "$TIMESCALEDB_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# Redis
echo -n "  Redis ($REDIS_HOST:$REDIS_PORT): "
if check_tcp_port "$REDIS_HOST" "$REDIS_PORT"; then
    # Try PING command
    if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" PING 2>/dev/null | grep -q "PONG"; then
        log_success "OK"
    else
        log_warn "Port open but PING failed"
    fi
else
    log_fail "Not reachable"
fi

# Kafka
echo -n "  Kafka ($KAFKA_BROKER): "
IFS=':' read -r kafka_host kafka_port <<< "$KAFKA_BROKER"
if check_tcp_port "$kafka_host" "$kafka_port"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# Nacos
echo -n "  Nacos ($NACOS_HOST:$NACOS_PORT): "
if check_http_endpoint "http://$NACOS_HOST:$NACOS_PORT/nacos/v1/console/health/readiness"; then
    log_success "OK"
else
    if check_tcp_port "$NACOS_HOST" "$NACOS_PORT"; then
        log_warn "Port open but health check failed"
    else
        log_fail "Not reachable"
    fi
fi

echo ""

# ============================================================================
# Service Health Checks
# ============================================================================

log_info "Checking Application Services..."
echo ""

# eidos-api (HTTP)
echo -n "  eidos-api (HTTP :$API_PORT): "
if check_http_endpoint "http://localhost:$API_PORT/health" 200; then
    log_success "OK"
else
    if check_tcp_port "localhost" "$API_PORT"; then
        log_warn "Port open but health endpoint failed"
    else
        log_fail "Not reachable"
    fi
fi

# eidos-trading (gRPC)
echo -n "  eidos-trading (gRPC :$TRADING_PORT): "
if check_grpc_service "localhost" "$TRADING_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# eidos-matching (gRPC)
echo -n "  eidos-matching (gRPC :$MATCHING_PORT): "
if check_grpc_service "localhost" "$MATCHING_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# eidos-market (gRPC)
echo -n "  eidos-market (gRPC :$MARKET_PORT): "
if check_grpc_service "localhost" "$MARKET_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# eidos-chain (gRPC)
echo -n "  eidos-chain (gRPC :$CHAIN_PORT): "
if check_grpc_service "localhost" "$CHAIN_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# eidos-risk (gRPC)
echo -n "  eidos-risk (gRPC :$RISK_PORT): "
if check_grpc_service "localhost" "$RISK_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# eidos-jobs (gRPC)
echo -n "  eidos-jobs (gRPC :$JOBS_PORT): "
if check_grpc_service "localhost" "$JOBS_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

# eidos-admin (HTTP)
echo -n "  eidos-admin (HTTP :$ADMIN_PORT): "
if check_tcp_port "localhost" "$ADMIN_PORT"; then
    log_success "OK"
else
    log_fail "Not reachable"
fi

echo ""

# ============================================================================
# Monitoring Health Checks
# ============================================================================

log_info "Checking Monitoring Components..."
echo ""

# Prometheus
echo -n "  Prometheus (localhost:9090): "
if check_http_endpoint "http://localhost:9090/-/healthy" 200; then
    log_success "OK"
else
    if check_tcp_port "localhost" 9090; then
        log_warn "Port open but health check failed"
    else
        log_fail "Not reachable"
    fi
fi

# Grafana
echo -n "  Grafana (localhost:3000): "
if check_http_endpoint "http://localhost:3000/api/health" 200; then
    log_success "OK"
else
    if check_tcp_port "localhost" 3000; then
        log_warn "Port open but health check failed"
    else
        log_fail "Not reachable"
    fi
fi

# Kafka UI
echo -n "  Kafka UI (localhost:8090): "
if check_tcp_port "localhost" 8090; then
    log_success "OK"
else
    log_warn "Not available (optional)"
fi

echo ""

# ============================================================================
# Summary
# ============================================================================

echo "=============================================="
echo "  Health Check Summary"
echo "=============================================="
echo ""
echo -e "  ${GREEN}Passed:${NC}   $PASSED"
echo -e "  ${RED}Failed:${NC}   $FAILED"
echo -e "  ${YELLOW}Warnings:${NC} $WARNINGS"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All critical checks passed!${NC}"
    exit 0
else
    echo -e "${RED}Some critical checks failed. Please review the output above.${NC}"
    exit 1
fi
