#!/bin/bash

# Kafka server address
KAFKA_BOOTSTRAP_SERVER=${KAFKA_BOOTSTRAP_SERVER:-"kafka:9092"}
# Default Replication Factor (1 for dev/docker, 3 for prod)
REPLICATION_FACTOR=${REPLICATION_FACTOR:-1}

echo "Waiting for Kafka to be ready at $KAFKA_BOOTSTRAP_SERVER..."
# Loop until Kafka is ready
until /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" --list > /dev/null 2>&1; do
  echo "Kafka is not ready yet..."
  sleep 2
done

echo "Kafka is ready! Creating topics..."

# Function to create topic if not exists
create_topic() {
  local topic_name=$1
  local partitions=$2

  echo "Creating topic: $topic_name (Partitions: $partitions, RF: $REPLICATION_FACTOR)"
  
  /opt/kafka/bin/kafka-topics.sh --create \
    --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" \
    --replication-factor "$REPLICATION_FACTOR" \
    --partitions "$partitions" \
    --topic "$topic_name" \
    --if-not-exists
}

# --- Core Trading Topics (Essential) ---
create_topic "orders" 6             # Trading -> Matching
create_topic "trade-results" 6      # Matching -> Trading
create_topic "cancel-requests" 3    # Trading -> Matching
create_topic "order-cancelled" 3    # Matching -> Trading

# --- Chain Integration Topics (Essential) ---
create_topic "deposits" 3              # Chain -> Trading
create_topic "withdrawals" 3           # Trading -> Chain
create_topic "withdrawal-confirmed" 3  # Chain -> Trading
create_topic "settlements" 3           # Trading -> Chain
create_topic "settlement-confirmed" 3  # Chain -> Trading

# --- Internal Audit Topics (Used by BalanceService) ---
create_topic "balance_log" 3        # BalanceService -> Data Warehouse (Audit)

# --- Future/External Service Topics (Enable when deploying relevant services) ---
create_topic "orderbook-updates" 6  # Matching -> Market
create_topic "order-updates" 6      # Trading -> API/WS
create_topic "balance-updates" 3    # Trading -> API/WS
create_topic "kline-1m" 3           # Market -> API/WS
create_topic "risk-alerts" 3        # Risk -> Monitor

echo "All topics created successfully."
