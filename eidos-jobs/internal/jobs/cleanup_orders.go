package jobs

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"go.uber.org/zap"
)

// OrderExpireRequest 订单过期请求
type OrderExpireRequest struct {
	OrderID string
	Market  string
	Wallet  string
}

// MatchingClient 撮合服务客户端接口
type MatchingClient interface {
	// ExpireOrders 发送过期订单请求到撮合引擎
	ExpireOrders(ctx context.Context, requests []*OrderExpireRequest) (int, error)
}

// TradingClient 交易服务客户端接口
type TradingClient interface {
	// GetExpiredOrders 获取需要过期的订单
	GetExpiredOrders(ctx context.Context, beforeTime int64, limit int) ([]*OrderExpireRequest, error)
}

// CleanupOrdersJob 清理过期订单任务
type CleanupOrdersJob struct {
	scheduler.BaseJob
	tradingClient  TradingClient
	matchingClient MatchingClient
	batchSize      int
}

// NewCleanupOrdersJob 创建清理过期订单任务
func NewCleanupOrdersJob(
	tradingClient TradingClient,
	matchingClient MatchingClient,
) *CleanupOrdersJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameCleanupOrders]

	return &CleanupOrdersJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameCleanupOrders,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		tradingClient:  tradingClient,
		matchingClient: matchingClient,
		batchSize:      100,
	}
}

// Execute 执行清理过期订单
func (j *CleanupOrdersJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	// 获取当前时间作为过期判断基准
	now := time.Now().UnixMilli()

	totalProcessed := 0
	totalExpired := 0
	totalErrors := 0

	for {
		select {
		case <-ctx.Done():
			result.ProcessedCount = totalProcessed
			result.AffectedCount = totalExpired
			result.ErrorCount = totalErrors
			return result, ctx.Err()
		default:
		}

		// 从交易服务获取需要过期的订单
		expiredOrders, err := j.tradingClient.GetExpiredOrders(ctx, now, j.batchSize)
		if err != nil {
			logger.Error("failed to get expired orders", zap.Error(err))
			totalErrors++
			break
		}

		if len(expiredOrders) == 0 {
			break
		}

		totalProcessed += len(expiredOrders)

		// 发送到撮合引擎处理
		// 撮合引擎会:
		// 1. 检查订单簿中订单是否存在
		// 2. 移除订单簿中的订单
		// 3. 发送 order-cancelled Kafka 消息
		// 4. trading 服务消费消息后更新数据库状态和解冻余额
		affected, err := j.matchingClient.ExpireOrders(ctx, expiredOrders)
		if err != nil {
			logger.Error("failed to expire orders in matching engine", zap.Error(err))
			totalErrors++
			continue
		}

		totalExpired += affected

		logger.Debug("processed batch of expired orders",
			zap.Int("batch_size", len(expiredOrders)),
			zap.Int("expired", affected))

		// 如果处理的数量少于批次大小，说明已经处理完了
		if len(expiredOrders) < j.batchSize {
			break
		}
	}

	result.ProcessedCount = totalProcessed
	result.AffectedCount = totalExpired
	result.ErrorCount = totalErrors
	result.Details["expired_orders"] = totalExpired
	result.Details["processed_orders"] = totalProcessed

	logger.Info("cleanup orders completed",
		zap.Int("processed", totalProcessed),
		zap.Int("expired", totalExpired),
		zap.Int("errors", totalErrors))

	return result, nil
}

// MockTradingClient 模拟交易客户端 (用于测试)
type MockTradingClient struct{}

func (c *MockTradingClient) GetExpiredOrders(ctx context.Context, beforeTime int64, limit int) ([]*OrderExpireRequest, error) {
	// TODO: 实际实现通过 gRPC 调用 trading 服务
	return nil, nil
}

// MockMatchingClient 模拟撮合客户端 (用于测试)
type MockMatchingClient struct{}

func (c *MockMatchingClient) ExpireOrders(ctx context.Context, requests []*OrderExpireRequest) (int, error) {
	// TODO: 实际实现通过 gRPC 调用 matching 服务
	return len(requests), nil
}
