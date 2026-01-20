// Package client 提供外部服务 gRPC 客户端
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
	matchingpb "github.com/eidos-exchange/eidos/proto/matching/v1"
)

// MatchingClient 撮合服务客户端
type MatchingClient struct {
	conn   *grpc.ClientConn
	client matchingpb.MatchingServiceClient
	// Kafka producer for sending cancel requests
	kafkaProducer KafkaProducer
}

// KafkaProducer Kafka 生产者接口
type KafkaProducer interface {
	SendWithContext(ctx context.Context, topic string, key, value []byte) error
}

// NewMatchingClient 创建撮合服务客户端
func NewMatchingClient(addr string, kafkaProducer KafkaProducer) (*MatchingClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to matching service: %w", err)
	}

	logger.Info("connected to eidos-matching", zap.String("addr", addr))

	return &MatchingClient{
		conn:          conn,
		client:        matchingpb.NewMatchingServiceClient(conn),
		kafkaProducer: kafkaProducer,
	}, nil
}

// CancelRequestMessage 取消请求消息 (对应 proto/matching/v1/matching.proto)
type CancelRequestMessage struct {
	OrderID     string `json:"order_id"`
	Market      string `json:"market"`
	Wallet      string `json:"wallet"`
	RequestedAt int64  `json:"requested_at"`
}

// ExpireOrders 发送过期订单请求到撮合引擎
// 通过 Kafka 发送取消请求消息
func (c *MatchingClient) ExpireOrders(ctx context.Context, requests []*jobs.OrderExpireRequest) (int, error) {
	if c.kafkaProducer == nil {
		logger.Warn("kafka producer not initialized, skipping expire orders")
		return 0, nil
	}

	now := time.Now().UnixMilli()
	successCount := 0

	for _, req := range requests {
		msg := &CancelRequestMessage{
			OrderID:     req.OrderID,
			Market:      req.Market,
			Wallet:      req.Wallet,
			RequestedAt: now,
		}

		data, err := json.Marshal(msg)
		if err != nil {
			logger.Error("marshal cancel request",
				zap.String("order_id", req.OrderID),
				zap.Error(err))
			continue
		}

		// 发送到 cancel-requests topic
		if err := c.kafkaProducer.SendWithContext(ctx, "cancel-requests", []byte(req.OrderID), data); err != nil {
			logger.Error("send cancel request to kafka",
				zap.String("order_id", req.OrderID),
				zap.Error(err))
			continue
		}

		successCount++
	}

	logger.Info("expire orders sent",
		zap.Int("total", len(requests)),
		zap.Int("success", successCount))

	return successCount, nil
}

// HealthCheck 健康检查
func (c *MatchingClient) HealthCheck(ctx context.Context) error {
	resp, err := c.client.HealthCheck(ctx, &matchingpb.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	if !resp.Healthy {
		return fmt.Errorf("matching service unhealthy")
	}

	return nil
}

// Close 关闭连接
func (c *MatchingClient) Close() error {
	if c.conn != nil {
		logger.Info("closing matching client connection")
		return c.conn.Close()
	}
	return nil
}

// Ensure MatchingClient implements jobs.MatchingClient
var _ jobs.MatchingClient = (*MatchingClient)(nil)
