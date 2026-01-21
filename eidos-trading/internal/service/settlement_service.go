// Package service provides settlement batch management
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// Settlement batch errors
var (
	ErrBatchNotFound      = errors.New("settlement batch not found")
	ErrBatchAlreadyExists = errors.New("settlement batch already exists")
	ErrBatchNotPending    = errors.New("settlement batch is not in pending status")
	ErrNoTradesToSettle   = errors.New("no trades to settle")
	ErrBatchSizeExceeded  = errors.New("batch size exceeded")
)

// Settlement batch status
const (
	BatchStatusPending   = "pending"   // 待处理
	BatchStatusSubmitted = "submitted" // 已提交到链上
	BatchStatusConfirmed = "confirmed" // 链上已确认
	BatchStatusFailed    = "failed"    // 失败
	BatchStatusRolledBack = "rolled_back" // 已回滚
)

// Default batch settings
const (
	DefaultBatchSize     = 100               // 默认批次大小
	MaxBatchSize         = 500               // 最大批次大小
	BatchTimeoutDuration = 30 * time.Minute  // 批次超时时间
)

// SettlementBatch 结算批次
type SettlementBatch struct {
	BatchID     string          `json:"batch_id" gorm:"primaryKey;type:varchar(64)"`
	TradeIDs    []string        `json:"trade_ids" gorm:"-"`                           // 不存储在 DB，使用 TradeIDsJSON
	TradeIDsJSON string         `json:"-" gorm:"type:text;column:trade_ids"`
	TradeCount  int             `json:"trade_count" gorm:"type:int"`
	TotalAmount decimal.Decimal `json:"total_amount" gorm:"type:decimal(36,18)"`      // 批次总金额 (Quote Token)
	Status      string          `json:"status" gorm:"type:varchar(20);index"`
	TxHash      string          `json:"tx_hash" gorm:"type:varchar(66)"`
	BlockNumber int64           `json:"block_number" gorm:"type:bigint"`
	ErrorMsg    string          `json:"error_msg" gorm:"type:text"`
	CreatedAt   int64           `json:"created_at" gorm:"type:bigint;autoCreateTime:milli"`
	UpdatedAt   int64           `json:"updated_at" gorm:"type:bigint;autoUpdateTime:milli"`
	SubmittedAt int64           `json:"submitted_at" gorm:"type:bigint"`
	ConfirmedAt int64           `json:"confirmed_at" gorm:"type:bigint"`
}

// TableName 返回表名
func (SettlementBatch) TableName() string {
	return "settlement_batches"
}

// BeforeSave 保存前序列化 TradeIDs
func (b *SettlementBatch) BeforeSave(tx *gorm.DB) error {
	if len(b.TradeIDs) > 0 {
		data, err := json.Marshal(b.TradeIDs)
		if err != nil {
			return err
		}
		b.TradeIDsJSON = string(data)
	}
	return nil
}

// AfterFind 查询后反序列化 TradeIDs
func (b *SettlementBatch) AfterFind(tx *gorm.DB) error {
	if b.TradeIDsJSON != "" {
		return json.Unmarshal([]byte(b.TradeIDsJSON), &b.TradeIDs)
	}
	return nil
}

// SettlementService 结算服务接口
type SettlementService interface {
	// CreateBatch 创建结算批次
	CreateBatch(ctx context.Context, tradeIDs []string) (*SettlementBatch, error)

	// GetBatch 获取批次详情
	GetBatch(ctx context.Context, batchID string) (*SettlementBatch, error)

	// ListPendingBatches 获取待处理批次
	ListPendingBatches(ctx context.Context, limit int) ([]*SettlementBatch, error)

	// MarkBatchSubmitted 标记批次已提交
	MarkBatchSubmitted(ctx context.Context, batchID, txHash string) error

	// MarkBatchConfirmed 标记批次已确认
	MarkBatchConfirmed(ctx context.Context, batchID, txHash string, blockNumber int64) error

	// MarkBatchFailed 标记批次失败
	MarkBatchFailed(ctx context.Context, batchID, errorMsg string) error

	// TriggerSettlement 触发结算 (供 eidos-jobs 调用)
	// 扫描待结算的成交，创建批次并发送到 eidos-chain
	TriggerSettlement(ctx context.Context, batchSize int) (*SettlementBatch, error)

	// RetryFailedBatches 重试失败的批次
	RetryFailedBatches(ctx context.Context, limit int) (int, error)

	// GetUnconfirmedBatches 获取已提交但未确认的批次 (用于超时检测)
	GetUnconfirmedBatches(ctx context.Context, submittedBefore int64, limit int) ([]*SettlementBatch, error)
}

// settlementService 结算服务实现
type settlementService struct {
	db                  *gorm.DB
	tradeRepo           TradeRepository
	idGenerator         IDGenerator
	settlementPublisher SettlementPublisher
}

// TradeRepository 成交仓储接口 (用于结算服务)
type TradeRepository interface {
	GetByTradeID(ctx context.Context, tradeID string) (*model.Trade, error)
	ListUnsettledTrades(ctx context.Context, limit int) ([]*model.Trade, error)
	UpdateSettlementBatchID(ctx context.Context, tradeID, batchID string) error
}

// SettlementPublisher 结算消息发布接口
type SettlementPublisher interface {
	PublishSettlementTrade(ctx context.Context, trade *model.Trade) error
	PublishSettlementTradeFromResult(
		ctx context.Context,
		tradeID, market, makerWallet, takerWallet string,
		makerOrderID, takerOrderID string,
		price, amount, quoteAmount, makerFee, takerFee decimal.Decimal,
		makerIsBuyer bool,
		matchedAt int64,
	) error
}

// NewSettlementService 创建结算服务
func NewSettlementService(
	db *gorm.DB,
	tradeRepo TradeRepository,
	idGenerator IDGenerator,
	settlementPublisher SettlementPublisher,
) SettlementService {
	return &settlementService{
		db:                  db,
		tradeRepo:           tradeRepo,
		idGenerator:         idGenerator,
		settlementPublisher: settlementPublisher,
	}
}

// CreateBatch 创建结算批次
func (s *settlementService) CreateBatch(ctx context.Context, tradeIDs []string) (*SettlementBatch, error) {
	if len(tradeIDs) == 0 {
		return nil, ErrNoTradesToSettle
	}

	if len(tradeIDs) > MaxBatchSize {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrBatchSizeExceeded, len(tradeIDs), MaxBatchSize)
	}

	// 生成批次 ID
	batchIDInt, err := s.idGenerator.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate batch id: %w", err)
	}
	batchID := fmt.Sprintf("SB%d", batchIDInt)

	// 计算批次总金额
	var totalAmount decimal.Decimal
	for _, tradeID := range tradeIDs {
		trade, err := s.tradeRepo.GetByTradeID(ctx, tradeID)
		if err != nil {
			logger.Warn("get trade for batch calculation failed",
				"trade_id", tradeID,
				"error", err)
			continue
		}
		totalAmount = totalAmount.Add(trade.QuoteAmount)
	}

	now := time.Now().UnixMilli()
	batch := &SettlementBatch{
		BatchID:     batchID,
		TradeIDs:    tradeIDs,
		TradeCount:  len(tradeIDs),
		TotalAmount: totalAmount,
		Status:      BatchStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 创建批次记录
	if err := s.db.WithContext(ctx).Create(batch).Error; err != nil {
		return nil, fmt.Errorf("create settlement batch: %w", err)
	}

	// 更新成交记录的批次 ID
	for _, tradeID := range tradeIDs {
		if err := s.tradeRepo.UpdateSettlementBatchID(ctx, tradeID, batchID); err != nil {
			logger.Warn("update trade batch id failed",
				"trade_id", tradeID,
				"batch_id", batchID,
				"error", err)
		}
	}

	logger.Info("settlement batch created",
		"batch_id", batchID,
		"trade_count", len(tradeIDs),
		"total_amount", totalAmount.String())

	metrics.RecordSettlement("batch_created")

	return batch, nil
}

// GetBatch 获取批次详情
func (s *settlementService) GetBatch(ctx context.Context, batchID string) (*SettlementBatch, error) {
	var batch SettlementBatch
	if err := s.db.WithContext(ctx).Where("batch_id = ?", batchID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("get settlement batch: %w", err)
	}
	return &batch, nil
}

// ListPendingBatches 获取待处理批次
func (s *settlementService) ListPendingBatches(ctx context.Context, limit int) ([]*SettlementBatch, error) {
	var batches []*SettlementBatch
	if err := s.db.WithContext(ctx).
		Where("status = ?", BatchStatusPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&batches).Error; err != nil {
		return nil, fmt.Errorf("list pending batches: %w", err)
	}
	return batches, nil
}

// MarkBatchSubmitted 标记批次已提交
func (s *settlementService) MarkBatchSubmitted(ctx context.Context, batchID, txHash string) error {
	now := time.Now().UnixMilli()
	result := s.db.WithContext(ctx).Model(&SettlementBatch{}).
		Where("batch_id = ? AND status = ?", batchID, BatchStatusPending).
		Updates(map[string]interface{}{
			"status":       BatchStatusSubmitted,
			"tx_hash":      txHash,
			"submitted_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark batch submitted: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrBatchNotPending
	}

	logger.Info("settlement batch submitted",
		"batch_id", batchID,
		"tx_hash", txHash)

	metrics.RecordSettlement("batch_submitted")
	return nil
}

// MarkBatchConfirmed 标记批次已确认
func (s *settlementService) MarkBatchConfirmed(ctx context.Context, batchID, txHash string, blockNumber int64) error {
	now := time.Now().UnixMilli()
	result := s.db.WithContext(ctx).Model(&SettlementBatch{}).
		Where("batch_id = ? AND status = ?", batchID, BatchStatusSubmitted).
		Updates(map[string]interface{}{
			"status":       BatchStatusConfirmed,
			"tx_hash":      txHash,
			"block_number": blockNumber,
			"confirmed_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark batch confirmed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// 可能已经是 confirmed 状态 (幂等)
		batch, err := s.GetBatch(ctx, batchID)
		if err != nil {
			return err
		}
		if batch.Status == BatchStatusConfirmed {
			return nil // 幂等
		}
		return fmt.Errorf("batch status is %s, expected submitted", batch.Status)
	}

	logger.Info("settlement batch confirmed",
		"batch_id", batchID,
		"tx_hash", txHash,
		"block_number", blockNumber)

	metrics.RecordSettlement("batch_confirmed")
	return nil
}

// MarkBatchFailed 标记批次失败
func (s *settlementService) MarkBatchFailed(ctx context.Context, batchID, errorMsg string) error {
	now := time.Now().UnixMilli()
	result := s.db.WithContext(ctx).Model(&SettlementBatch{}).
		Where("batch_id = ?", batchID).
		Updates(map[string]interface{}{
			"status":     BatchStatusFailed,
			"error_msg":  errorMsg,
			"updated_at": now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark batch failed: %w", result.Error)
	}

	logger.Warn("settlement batch failed",
		"batch_id", batchID,
		"error", errorMsg)

	metrics.RecordSettlement("batch_failed")
	return nil
}

// TriggerSettlement 触发结算 (供 eidos-jobs 调用)
func (s *settlementService) TriggerSettlement(ctx context.Context, batchSize int) (*SettlementBatch, error) {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	if batchSize > MaxBatchSize {
		batchSize = MaxBatchSize
	}

	// 1. 获取待结算的成交
	trades, err := s.tradeRepo.ListUnsettledTrades(ctx, batchSize)
	if err != nil {
		return nil, fmt.Errorf("list unsettled trades: %w", err)
	}

	if len(trades) == 0 {
		return nil, ErrNoTradesToSettle
	}

	// 2. 提取成交 ID
	tradeIDs := make([]string, len(trades))
	for i, trade := range trades {
		tradeIDs[i] = trade.TradeID
	}

	// 3. 创建批次
	batch, err := s.CreateBatch(ctx, tradeIDs)
	if err != nil {
		return nil, err
	}

	// 4. 发送结算消息到 eidos-chain
	if s.settlementPublisher != nil {
		for _, trade := range trades {
			if err := s.settlementPublisher.PublishSettlementTradeFromResult(
				ctx,
				trade.TradeID,
				trade.Market,
				trade.MakerWallet,
				trade.TakerWallet,
				trade.MakerOrderID,
				trade.TakerOrderID,
				trade.Price,
				trade.Amount,
				trade.QuoteAmount,
				trade.MakerFee,
				trade.TakerFee,
				trade.MakerSide == model.OrderSideBuy,
				trade.MatchedAt,
			); err != nil {
				logger.Error("publish settlement trade failed",
					"trade_id", trade.TradeID,
					"batch_id", batch.BatchID,
					"error", err)
			}
		}
	}

	logger.Info("settlement triggered",
		"batch_id", batch.BatchID,
		"trade_count", len(trades))

	return batch, nil
}

// RetryFailedBatches 重试失败的批次
func (s *settlementService) RetryFailedBatches(ctx context.Context, limit int) (int, error) {
	// 获取失败的批次
	var batches []*SettlementBatch
	if err := s.db.WithContext(ctx).
		Where("status = ?", BatchStatusFailed).
		Order("created_at ASC").
		Limit(limit).
		Find(&batches).Error; err != nil {
		return 0, fmt.Errorf("list failed batches: %w", err)
	}

	if len(batches) == 0 {
		return 0, nil
	}

	retried := 0
	for _, batch := range batches {
		// 重置状态为 pending
		now := time.Now().UnixMilli()
		if err := s.db.WithContext(ctx).Model(&SettlementBatch{}).
			Where("batch_id = ? AND status = ?", batch.BatchID, BatchStatusFailed).
			Updates(map[string]interface{}{
				"status":     BatchStatusPending,
				"error_msg":  "",
				"updated_at": now,
			}).Error; err != nil {
			logger.Error("retry failed batch failed",
				"batch_id", batch.BatchID,
				"error", err)
			continue
		}

		// 重新发送结算消息
		for _, tradeID := range batch.TradeIDs {
			trade, err := s.tradeRepo.GetByTradeID(ctx, tradeID)
			if err != nil {
				continue
			}
			if s.settlementPublisher != nil {
				_ = s.settlementPublisher.PublishSettlementTradeFromResult(
					ctx,
					trade.TradeID,
					trade.Market,
					trade.MakerWallet,
					trade.TakerWallet,
					trade.MakerOrderID,
					trade.TakerOrderID,
					trade.Price,
					trade.Amount,
					trade.QuoteAmount,
					trade.MakerFee,
					trade.TakerFee,
					trade.MakerSide == model.OrderSideBuy,
					trade.MatchedAt,
				)
			}
		}

		retried++
		logger.Info("settlement batch retried",
			"batch_id", batch.BatchID,
			"trade_count", batch.TradeCount)
	}

	return retried, nil
}

// GetUnconfirmedBatches 获取已提交但未确认的批次
func (s *settlementService) GetUnconfirmedBatches(ctx context.Context, submittedBefore int64, limit int) ([]*SettlementBatch, error) {
	var batches []*SettlementBatch
	if err := s.db.WithContext(ctx).
		Where("status = ? AND submitted_at < ?", BatchStatusSubmitted, submittedBefore).
		Order("submitted_at ASC").
		Limit(limit).
		Find(&batches).Error; err != nil {
		return nil, fmt.Errorf("list unconfirmed batches: %w", err)
	}
	return batches, nil
}
