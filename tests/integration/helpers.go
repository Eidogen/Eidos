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

// TestHelper 集成测试辅助工具
type TestHelper struct {
	config      *TestConfig
	grpcConns   map[string]*grpc.ClientConn
	redisClient *redis.Client
	kafkaClient sarama.Client
}

// NewTestHelper 创建测试辅助工具实例
func NewTestHelper(config *TestConfig) *TestHelper {
	return &TestHelper{
		config:    config,
		grpcConns: make(map[string]*grpc.ClientConn),
	}
}

// Setup 初始化所有连接
func (h *TestHelper) Setup(ctx context.Context) error {
	// 建立 Redis 连接
	h.redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", h.config.RedisHost, h.config.RedisPort),
		Password: h.config.RedisPassword,
		DB:       0,
	})

	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("连接 Redis 失败: %w", err)
	}

	// 建立 Kafka 连接
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = sarama.V3_5_0_0
	kafkaClient, err := sarama.NewClient(h.config.KafkaBrokers, kafkaConfig)
	if err != nil {
		return fmt.Errorf("连接 Kafka 失败: %w", err)
	}
	h.kafkaClient = kafkaClient

	return nil
}

// Cleanup 关闭所有连接
func (h *TestHelper) Cleanup() error {
	var errs []error

	for name, conn := range h.grpcConns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 gRPC 连接 %s 失败: %w", name, err))
		}
	}

	if h.redisClient != nil {
		if err := h.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Redis 连接失败: %w", err))
		}
	}

	if h.kafkaClient != nil {
		if err := h.kafkaClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 Kafka 连接失败: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("清理错误: %v", errs)
	}
	return nil
}

// GetGRPCConnection 获取指定服务的 gRPC 连接
func (h *TestHelper) GetGRPCConnection(ctx context.Context, serviceName, addr string) (*grpc.ClientConn, error) {
	if conn, ok := h.grpcConns[serviceName]; ok {
		return conn, nil
	}

	// 使用 NewClient 创建非阻塞连接
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("连接 %s (%s) 失败: %w", serviceName, addr, err)
	}

	h.grpcConns[serviceName] = conn
	return conn, nil
}

// GetRedisClient 返回 Redis 客户端
func (h *TestHelper) GetRedisClient() *redis.Client {
	return h.redisClient
}

// GetKafkaClient 返回 Kafka 客户端
func (h *TestHelper) GetKafkaClient() sarama.Client {
	return h.kafkaClient
}

// GenerateOrderID 生成唯一订单 ID
func GenerateOrderID() string {
	return uuid.New().String()
}

// GenerateWalletAddress 生成随机钱包地址 (用于测试)
func GenerateWalletAddress() string {
	bytes := make([]byte, 20)
	_, _ = rand.Read(bytes)
	return "0x" + hex.EncodeToString(bytes)
}

// GenerateTxHash 生成随机交易哈希 (用于测试)
// 返回 66 字符的字符串 (0x + 64 个十六进制字符)
func GenerateTxHash() string {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	return "0x" + hex.EncodeToString(bytes)
}

// GenerateNonce 生成唯一 nonce (用于订单签名)
func GenerateNonce() uint64 {
	return uint64(time.Now().UnixNano())
}

// WaitForCondition 等待条件满足，支持重试
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
	return fmt.Errorf("重试 %d 次后条件仍未满足", maxRetries)
}

// MockSignature 生成模拟的 EIP-712 签名 (用于测试)
// 生产环境中应使用真实的加密签名
func MockSignature() []byte {
	// 生成 65 字节签名 (r: 32 字节, s: 32 字节, v: 1 字节)
	sig := make([]byte, 65)
	_, _ = rand.Read(sig[:64])
	sig[64] = 27 // v 值
	return sig
}

// FormatDecimal 格式化小数值为字符串
func FormatDecimal(value float64, precision int) string {
	return fmt.Sprintf("%.*f", precision, value)
}

// SetBalance 直接在 Redis 中设置余额 (用于测试)
// 此方法绕过充值流程，直接给账户充值
func (h *TestHelper) SetBalance(ctx context.Context, wallet, token, amount string) error {
	key := fmt.Sprintf("eidos:trading:balance:%s:%s", wallet, token)

	// 设置余额哈希字段 (包含所有必需字段)
	return h.redisClient.HSet(ctx, key, map[string]interface{}{
		"settled_available": amount,
		"settled_frozen":    "0",
		"pending_available": "0",
		"pending_frozen":    "0",
		"pending_total":     "0",
		"version":           1,
		"updated_at":        time.Now().Unix(),
	}).Err()
}
