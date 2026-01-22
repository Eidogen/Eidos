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

# Service ports (gRPC)
API_PORT="${API_PORT:-8080}"
TRADING_GRPC_PORT="${TRADING_GRPC_PORT:-50051}"
MATCHING_GRPC_PORT="${MATCHING_GRPC_PORT:-50052}"
MARKET_GRPC_PORT="${MARKET_GRPC_PORT:-50053}"
CHAIN_GRPC_PORT="${CHAIN_GRPC_PORT:-50054}"
RISK_GRPC_PORT="${RISK_GRPC_PORT:-50055}"
JOBS_GRPC_PORT="${JOBS_GRPC_PORT:-50056}"
ADMIN_PORT="${ADMIN_PORT:-8088}"

# Service HTTP ports (for health checks)
TRADING_HTTP_PORT="${TRADING_HTTP_PORT:-8081}"
MATCHING_HTTP_PORT="${MATCHING_HTTP_PORT:-8082}"
MARKET_HTTP_PORT="${MARKET_HTTP_PORT:-8083}"
CHAIN_HTTP_PORT="${CHAIN_HTTP_PORT:-8084}"
RISK_HTTP_PORT="${RISK_HTTP_PORT:-8085}"
JOBS_HTTP_PORT="${JOBS_HTTP_PORT:-8086}"

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
log_info "Verifying PostgreSQL databases..."
for db in eidos eidos_trading eidos_chain eidos_risk eidos_jobs eidos_admin; do echo -n "  DB $db: "; if PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$db" -c "SELECT 1" >/dev/null 2>&1; then log_success "OK"; else log_warn "Failed to connect"; fi done

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

# eidos-trading (gRPC + HTTP health)
echo -n "  eidos-trading (gRPC :$TRADING_GRPC_PORT): "
if check_grpc_service "localhost" "$TRADING_GRPC_PORT"; then
    # Also check HTTP readiness endpoint
    if check_http_endpoint "http://localhost:$TRADING_HTTP_PORT/health/ready" 200; then
        log_success "OK (DB+Redis ready)"
    else
        log_warn "gRPC OK but DB/Redis not ready"
    fi
else
    log_fail "Not reachable"
fi

# eidos-matching (gRPC + HTTP health)
echo -n "  eidos-matching (gRPC :$MATCHING_GRPC_PORT): "
if check_grpc_service "localhost" "$MATCHING_GRPC_PORT"; then
    if check_http_endpoint "http://localhost:$MATCHING_HTTP_PORT/health/ready" 200; then
        log_success "OK (ready)"
    else
        log_warn "gRPC OK but not fully ready"
    fi
else
    log_fail "Not reachable"
fi

# eidos-market (gRPC + HTTP health)
echo -n "  eidos-market (gRPC :$MARKET_GRPC_PORT): "
if check_grpc_service "localhost" "$MARKET_GRPC_PORT"; then
    if check_http_endpoint "http://localhost:$MARKET_HTTP_PORT/health/ready" 200; then
        log_success "OK (DB+Redis ready)"
    else
        log_warn "gRPC OK but DB/Redis not ready"
    fi
else
    log_fail "Not reachable"
fi

# eidos-chain (gRPC + HTTP health)
echo -n "  eidos-chain (gRPC :$CHAIN_GRPC_PORT): "
if check_grpc_service "localhost" "$CHAIN_GRPC_PORT"; then
    if check_http_endpoint "http://localhost:$CHAIN_HTTP_PORT/health/ready" 200; then
        log_success "OK (DB+Redis ready)"
    else
        log_warn "gRPC OK but DB/Redis not ready"
    fi
else
    log_fail "Not reachable"
fi

# eidos-risk (gRPC + HTTP health)
echo -n "  eidos-risk (gRPC :$RISK_GRPC_PORT): "
if check_grpc_service "localhost" "$RISK_GRPC_PORT"; then
    if check_http_endpoint "http://localhost:$RISK_HTTP_PORT/health/ready" 200; then
        log_success "OK (DB+Redis ready)"
    else
        log_warn "gRPC OK but DB/Redis not ready"
    fi
else
    log_fail "Not reachable"
fi

# eidos-jobs (gRPC + HTTP health)
echo -n "  eidos-jobs (gRPC :$JOBS_GRPC_PORT): "
if check_grpc_service "localhost" "$JOBS_GRPC_PORT"; then
    if check_http_endpoint "http://localhost:$JOBS_HTTP_PORT/health/ready" 200; then
        log_success "OK (DB+Redis ready)"
    else
        log_warn "gRPC OK but DB/Redis not ready"
    fi
else
    log_fail "Not reachable"
fi

# eidos-admin (HTTP)
echo -n "  eidos-admin (HTTP :$ADMIN_PORT): "
if check_http_endpoint "http://localhost:$ADMIN_PORT/metrics" 200; then
    log_success "OK"
else
    if check_tcp_port "localhost" "$ADMIN_PORT"; then
        log_warn "Port open but metrics endpoint failed"
    else
        log_fail "Not reachable"
    fi
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

# Jaeger
echo -n "  Jaeger (localhost:16686): "
if check_http_endpoint "http://localhost:14269/" 200; then
    log_success "OK"
else
    if check_tcp_port "localhost" 16686; then
        log_warn "Port open but admin health check failed"
    else
        log_warn "Not available (optional for dev)"
    fi
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
