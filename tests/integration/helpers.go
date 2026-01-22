package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestHelper provides utility functions for integration tests.
type TestHelper struct {
	config      *TestConfig
	grpcConns   map[string]*grpc.ClientConn
	redisClient *redis.Client
	kafkaClient sarama.Client
}

// NewTestHelper creates a new test helper instance.
func NewTestHelper(config *TestConfig) *TestHelper {
	return &TestHelper{
		config:    config,
		grpcConns: make(map[string]*grpc.ClientConn),
	}
}

// Setup initializes all connections.
func (h *TestHelper) Setup(ctx context.Context) error {
	// Setup Redis connection
	h.redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", h.config.RedisHost, h.config.RedisPort),
		Password: h.config.RedisPassword,
		DB:       0,
	})

	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Setup Kafka connection
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = sarama.V3_5_0_0
	kafkaClient, err := sarama.NewClient(h.config.KafkaBrokers, kafkaConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	h.kafkaClient = kafkaClient

	return nil
}

// Cleanup closes all connections.
func (h *TestHelper) Cleanup() error {
	var errs []error

	for name, conn := range h.grpcConns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close gRPC connection %s: %w", name, err))
		}
	}

	if h.redisClient != nil {
		if err := h.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Redis connection: %w", err))
		}
	}

	if h.kafkaClient != nil {
		if err := h.kafkaClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Kafka connection: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}

// GetGRPCConnection returns a gRPC connection to the specified service.
func (h *TestHelper) GetGRPCConnection(ctx context.Context, serviceName, addr string) (*grpc.ClientConn, error) {
	if conn, ok := h.grpcConns[serviceName]; ok {
		return conn, nil
	}

	// Use NewClient instead of DialContext for non-blocking connection
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s at %s: %w", serviceName, addr, err)
	}

	h.grpcConns[serviceName] = conn
	return conn, nil
}

// GetRedisClient returns the Redis client.
func (h *TestHelper) GetRedisClient() *redis.Client {
	return h.redisClient
}

// GetKafkaClient returns the Kafka client.
func (h *TestHelper) GetKafkaClient() sarama.Client {
	return h.kafkaClient
}

// GenerateOrderID generates a unique order ID.
func GenerateOrderID() string {
	return uuid.New().String()
}

// GenerateWalletAddress generates a random wallet address for testing.
func GenerateWalletAddress() string {
	bytes := make([]byte, 20)
	_, _ = rand.Read(bytes)
	return "0x" + hex.EncodeToString(bytes)
}

// GenerateTxHash generates a random transaction hash for testing.
// Returns a 66-character string (0x + 64 hex characters).
func GenerateTxHash() string {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	return "0x" + hex.EncodeToString(bytes)
}

// GenerateNonce generates a unique nonce for order signing.
func GenerateNonce() uint64 {
	return uint64(time.Now().UnixNano())
}

// WaitForCondition waits for a condition to be true with retries.
func WaitForCondition(ctx context.Context, interval time.Duration, maxRetries int, condition func() (bool, error)) error {
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ok, err := condition()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		time.Sleep(interval)
	}
	return fmt.Errorf("condition not met after %d retries", maxRetries)
}

// MockSignature generates a mock EIP-712 signature for testing.
// In production, this would be a real cryptographic signature.
func MockSignature() []byte {
	// Generate a 65-byte signature (r: 32 bytes, s: 32 bytes, v: 1 byte)
	sig := make([]byte, 65)
	_, _ = rand.Read(sig[:64])
	sig[64] = 27 // v value
	return sig
}

// FormatDecimal formats a decimal value as a string.
func FormatDecimal(value float64, precision int) string {
	return fmt.Sprintf("%.*f", precision, value)
}

// SetBalance directly sets a balance in Redis for testing.
// This bypasses the deposit flow and directly credits the account.
func (h *TestHelper) SetBalance(ctx context.Context, wallet, token, amount string) error {
	key := fmt.Sprintf("eidos:trading:balance:%s:%s", wallet, token)

	// Set the balance hash fields
	return h.redisClient.HSet(ctx, key, map[string]interface{}{
		"settled_available": amount,
		"settled_frozen":    "0",
		"pending_available": "0",
		"pending_frozen":    "0",
	}).Err()
}
