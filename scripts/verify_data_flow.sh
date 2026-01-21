#!/bin/bash
#
# Eidos Trading System - Data Flow Verification Script
# This script verifies the data flow between all services
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
KAFKA_BROKER="${KAFKA_BROKER:-localhost:29092}"
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-eidos}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-eidos123}"
POSTGRES_DB="${POSTGRES_DB:-eidos}"

# Counters
PASSED=0
FAILED=0

# Helper functions
log_section() {
    echo ""
    echo -e "${CYAN}=== $1 ===${NC}"
    echo ""
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_check() {
    echo -n "  $1: "
}

log_success() {
    echo -e "${GREEN}OK${NC} - $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}FAIL${NC} - $1"
    ((FAILED++))
}

log_skip() {
    echo -e "${YELLOW}SKIP${NC} - $1"
}

# ============================================================================
# Kafka Topic Verification
# ============================================================================

verify_kafka_topics() {
    log_section "Kafka Topics Verification"

    # Expected topics based on the architecture
    EXPECTED_TOPICS=(
        "orders"
        "cancel-requests"
        "trade-results"
        "order-updates"
        "order-cancelled"
        "orderbook-updates"
        "balance-updates"
        "deposits"
        "deposit-confirmed"
        "withdrawals"
        "withdrawal-status"
        "settlements"
        "settlement-confirmed"
        "risk-alerts"
        "notifications"
        "market-stats"
    )

    log_info "Checking Kafka broker connectivity..."

    IFS=':' read -r kafka_host kafka_port <<< "$KAFKA_BROKER"
    if ! nc -z -w 5 "$kafka_host" "$kafka_port" 2>/dev/null; then
        log_fail "Cannot connect to Kafka broker at $KAFKA_BROKER"
        return
    fi

    log_info "Listing existing topics..."

    # Get list of topics
    EXISTING_TOPICS=$(kafka-topics.sh --bootstrap-server "$KAFKA_BROKER" --list 2>/dev/null || echo "")

    if [ -z "$EXISTING_TOPICS" ]; then
        log_fail "Could not retrieve topic list from Kafka"
        return
    fi

    for topic in "${EXPECTED_TOPICS[@]}"; do
        log_check "Topic: $topic"
        if echo "$EXISTING_TOPICS" | grep -q "^$topic$"; then
            # Get topic details
            PARTITIONS=$(kafka-topics.sh --bootstrap-server "$KAFKA_BROKER" --describe --topic "$topic" 2>/dev/null | grep "PartitionCount" | awk '{print $2}' || echo "?")
            log_success "Exists (partitions: $PARTITIONS)"
        else
            log_fail "Topic not found"
        fi
    done
}

# ============================================================================
# Kafka Consumer Group Verification
# ============================================================================

verify_kafka_consumer_groups() {
    log_section "Kafka Consumer Groups Verification"

    # Expected consumer groups
    EXPECTED_GROUPS=(
        "eidos-trading"
        "eidos-matching"
        "eidos-market"
        "eidos-chain"
        "eidos-risk"
        "eidos-jobs"
    )

    log_info "Listing consumer groups..."

    EXISTING_GROUPS=$(kafka-consumer-groups.sh --bootstrap-server "$KAFKA_BROKER" --list 2>/dev/null || echo "")

    if [ -z "$EXISTING_GROUPS" ]; then
        log_info "No consumer groups found (services may not be running)"
        return
    fi

    for group in "${EXPECTED_GROUPS[@]}"; do
        log_check "Consumer Group: $group"
        if echo "$EXISTING_GROUPS" | grep -q "^$group$"; then
            # Get lag info
            LAG=$(kafka-consumer-groups.sh --bootstrap-server "$KAFKA_BROKER" --describe --group "$group" 2>/dev/null | awk 'NR>1 {sum+=$5} END {print sum}' || echo "?")
            log_success "Active (lag: $LAG)"
        else
            log_skip "Group not active"
        fi
    done
}

# ============================================================================
# Redis Data Flow Verification
# ============================================================================

verify_redis_data() {
    log_section "Redis Data Flow Verification"

    log_info "Checking Redis connectivity..."

    if ! nc -z -w 5 "$REDIS_HOST" "$REDIS_PORT" 2>/dev/null; then
        log_fail "Cannot connect to Redis at $REDIS_HOST:$REDIS_PORT"
        return
    fi

    # Check for expected key patterns
    KEY_PATTERNS=(
        "orderbook:*"
        "ticker:*"
        "depth:*"
        "order:*"
        "balance:*"
        "rate_limit:*"
    )

    log_info "Checking Redis key patterns..."

    for pattern in "${KEY_PATTERNS[@]}"; do
        log_check "Pattern: $pattern"
        COUNT=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" KEYS "$pattern" 2>/dev/null | wc -l)
        if [ "$COUNT" -gt 0 ]; then
            log_success "Found $COUNT keys"
        else
            log_skip "No keys found (may be expected if no data yet)"
        fi
    done

    # Check Redis memory usage
    log_info "Redis memory info:"
    MEMORY=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" INFO memory 2>/dev/null | grep "used_memory_human" | cut -d':' -f2 | tr -d '\r')
    echo "  Used memory: $MEMORY"
}

# ============================================================================
# Database Schema Verification
# ============================================================================

verify_database_schema() {
    log_section "Database Schema Verification"

    log_info "Checking PostgreSQL connectivity..."

    if ! nc -z -w 5 "$POSTGRES_HOST" "$POSTGRES_PORT" 2>/dev/null; then
        log_fail "Cannot connect to PostgreSQL at $POSTGRES_HOST:$POSTGRES_PORT"
        return
    fi

    # Expected tables
    EXPECTED_TABLES=(
        "orders"
        "trades"
        "balances"
        "balance_logs"
        "deposits"
        "withdrawals"
        "outbox"
        "markets"
        "tokens"
    )

    log_info "Checking database tables..."

    for table in "${EXPECTED_TABLES[@]}"; do
        log_check "Table: $table"
        EXISTS=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "SELECT EXISTS(SELECT FROM information_schema.tables WHERE table_name='$table')" 2>/dev/null || echo "f")
        if [ "$EXISTS" == "t" ]; then
            COUNT=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "SELECT COUNT(*) FROM $table" 2>/dev/null || echo "?")
            log_success "Exists ($COUNT rows)"
        else
            log_fail "Table not found"
        fi
    done
}

# ============================================================================
# Service Communication Verification
# ============================================================================

verify_service_communication() {
    log_section "Service Communication Verification"

    log_info "Testing gRPC service connectivity..."

    # Service endpoints
    declare -A SERVICES=(
        ["eidos-api"]="localhost:8080"
        ["eidos-trading"]="localhost:50051"
        ["eidos-matching"]="localhost:50052"
        ["eidos-market"]="localhost:50053"
        ["eidos-chain"]="localhost:50054"
        ["eidos-risk"]="localhost:50055"
        ["eidos-jobs"]="localhost:50056"
        ["eidos-admin"]="localhost:8088"
    )

    for service in "${!SERVICES[@]}"; do
        log_check "$service (${SERVICES[$service]})"
        IFS=':' read -r host port <<< "${SERVICES[$service]}"
        if nc -z -w 5 "$host" "$port" 2>/dev/null; then
            log_success "Reachable"
        else
            log_fail "Not reachable"
        fi
    done
}

# ============================================================================
# Data Flow Path Verification
# ============================================================================

verify_data_flow_paths() {
    log_section "Data Flow Path Verification"

    log_info "Verifying data flow paths..."
    echo ""

    echo "  Order Flow:"
    echo "    API -> Trading -> [Kafka:orders] -> Matching -> [Kafka:trade-results] -> Trading"
    echo ""

    echo "  Deposit Flow:"
    echo "    Chain Indexer -> [Kafka:deposits] -> Trading -> Balance Update"
    echo ""

    echo "  Withdrawal Flow:"
    echo "    API -> Trading -> [Kafka:withdrawals] -> Chain -> [Kafka:withdrawal-status]"
    echo ""

    echo "  Market Data Flow:"
    echo "    Matching -> [Kafka:orderbook-updates] -> Market -> [Redis:orderbook]"
    echo "    Matching -> [Kafka:trade-results] -> Market -> [TimescaleDB:klines]"
    echo ""

    echo "  Settlement Flow:"
    echo "    Trading -> [Kafka:settlements] -> Chain -> [Kafka:settlement-confirmed] -> Trading"
    echo ""

    log_info "Checking Kafka message flow..."

    # Check for recent messages in key topics
    FLOW_TOPICS=("orders" "trade-results" "orderbook-updates" "settlements")

    for topic in "${FLOW_TOPICS[@]}"; do
        log_check "Topic: $topic (recent activity)"
        # Get latest offset
        OFFSETS=$(kafka-run-class.sh kafka.tools.GetOffsetShell --broker-list "$KAFKA_BROKER" --topic "$topic" 2>/dev/null | head -1 || echo "")
        if [ -n "$OFFSETS" ]; then
            OFFSET=$(echo "$OFFSETS" | cut -d':' -f3)
            if [ "$OFFSET" -gt 0 ]; then
                log_success "$OFFSET messages total"
            else
                log_skip "No messages yet"
            fi
        else
            log_skip "Could not check offsets"
        fi
    done
}

# ============================================================================
# Outbox Pattern Verification
# ============================================================================

verify_outbox_pattern() {
    log_section "Outbox Pattern Verification"

    log_info "Checking outbox table status..."

    log_check "Outbox table"
    OUTBOX_STATUS=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "
        SELECT
            COUNT(*) as total,
            SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
            SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent,
            SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
        FROM outbox
    " 2>/dev/null || echo "")

    if [ -n "$OUTBOX_STATUS" ]; then
        log_success "Status: $OUTBOX_STATUS"
    else
        log_skip "Could not query outbox table"
    fi

    # Check for stuck messages
    log_check "Stuck messages (pending > 5 min)"
    STUCK=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "
        SELECT COUNT(*) FROM outbox
        WHERE status = 'pending'
        AND created_at < NOW() - INTERVAL '5 minutes'
    " 2>/dev/null || echo "?")

    if [ "$STUCK" == "0" ]; then
        log_success "None"
    else
        log_fail "$STUCK stuck messages found"
    fi
}

# ============================================================================
# Main Execution
# ============================================================================

echo ""
echo "=============================================="
echo "  Eidos Trading System - Data Flow Verification"
echo "=============================================="
echo ""
echo "  Kafka: $KAFKA_BROKER"
echo "  Redis: $REDIS_HOST:$REDIS_PORT"
echo "  PostgreSQL: $POSTGRES_HOST:$POSTGRES_PORT"
echo ""

# Run verifications
verify_kafka_topics
verify_kafka_consumer_groups
verify_redis_data
verify_database_schema
verify_service_communication
verify_data_flow_paths
verify_outbox_pattern

# Summary
log_section "Verification Summary"

echo -e "  ${GREEN}Passed:${NC} $PASSED"
echo -e "  ${RED}Failed:${NC} $FAILED"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All data flow verifications passed!${NC}"
    exit 0
else
    echo -e "${YELLOW}Some verifications failed. Review the output above.${NC}"
    exit 1
fi
