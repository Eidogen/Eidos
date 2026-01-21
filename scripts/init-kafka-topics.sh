#!/bin/bash
#
# Eidos Trading System - Kafka Topics Initialization Script
# Creates all required Kafka topics for the trading system
#

set -e

KAFKA_BOOTSTRAP_SERVER="${KAFKA_BOOTSTRAP_SERVER:-kafka:9092}"
REPLICATION_FACTOR="${REPLICATION_FACTOR:-1}"

echo "Waiting for Kafka to be ready..."
sleep 30

# Function to create topic if it doesn't exist
create_topic() {
    local topic=$1
    local partitions=$2
    local retention_hours=${3:-168}  # Default 7 days

    echo "Creating topic: $topic (partitions: $partitions, retention: ${retention_hours}h)"

    /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" \
        --create \
        --if-not-exists \
        --topic "$topic" \
        --partitions "$partitions" \
        --replication-factor "$REPLICATION_FACTOR" \
        --config retention.ms=$((retention_hours * 3600000)) \
        || true
}

echo "=============================================="
echo "  Creating Kafka Topics for Eidos"
echo "=============================================="
echo ""

# Order Flow Topics
echo "Creating Order Flow Topics..."
create_topic "orders" 16                    # High traffic: new orders
create_topic "cancel-requests" 8            # Cancel requests
create_topic "order-accepted" 16            # Order accepted by matching
create_topic "order-cancelled" 8            # Order cancelled
create_topic "order-rejected" 8             # Order rejected
create_topic "order-updates" 16             # Order status updates

# Trade Topics
echo "Creating Trade Topics..."
create_topic "trade-results" 16             # Matched trades (high traffic)

# Market Data Topics
echo "Creating Market Data Topics..."
create_topic "orderbook-updates" 16         # Real-time orderbook updates
create_topic "market-stats" 4               # Market statistics

# Balance Topics
echo "Creating Balance Topics..."
create_topic "balance-updates" 8            # Balance change notifications

# Deposit/Withdrawal Topics
echo "Creating Deposit/Withdrawal Topics..."
create_topic "deposits" 4                   # Deposit events from chain
create_topic "deposit-confirmed" 4          # Deposit confirmations
create_topic "withdrawals" 4                # Withdrawal requests
create_topic "withdrawal-submitted" 4       # Withdrawal submitted to chain
create_topic "withdrawal-confirmed" 4       # Withdrawal confirmed
create_topic "withdrawal-status" 4          # Withdrawal status updates

# Settlement Topics
echo "Creating Settlement Topics..."
create_topic "settlements" 8                # Settlement batches
create_topic "settlement-submitted" 4       # Settlement submitted to chain
create_topic "settlement-confirmed" 4       # Settlement confirmed
create_topic "settlement-failed" 4          # Settlement failures

# Risk Topics
echo "Creating Risk Topics..."
create_topic "risk-alerts" 4                # Risk alerts
create_topic "risk-checks" 8                # Risk check requests

# Notification Topics
echo "Creating Notification Topics..."
create_topic "notifications" 8              # User notifications

# Admin Topics
echo "Creating Admin Topics..."
create_topic "admin-commands" 2             # Admin commands
create_topic "audit-logs" 4                 # Audit logs

echo ""
echo "=============================================="
echo "  Topic Creation Complete"
echo "=============================================="

# List all topics
echo ""
echo "Current topics:"
/opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" --list

echo ""
echo "Done!"
