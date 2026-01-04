package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"go.uber.org/zap"
)

var (
	ErrInvalidTrade       = errors.New("invalid trade")
	ErrTradeAlreadyExists = errors.New("trade already exists")
	ErrSettlementFailed   = errors.New("settlement failed")
)

// TradeService 成交服务接口
// 注意: ProcessTradeResult 已移至 ClearingService，使用 Redis Lua 原子操作实现
// 本服务只负责查询和结算批次管理
type TradeService interface {
	// GetTrade 获取成交详情
	GetTrade(ctx context.Context, tradeID string) (*model.Trade, error)

	// ListTradesByOrder 获取订单相关成交
	ListTradesByOrder(ctx context.Context, orderID string) ([]*model.Trade, error)

	// ListTrades 获取用户成交列表
	ListTrades(ctx context.Context, wallet string, filter *repository.TradeFilter, page *repository.Pagination) ([]*model.Trade, error)

	// ListRecentTrades 获取市场最近成交 (用于行情)
	ListRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error)

	// CreateSettlementBatch 创建结算批次
	// 将多笔成交打包为一个批次提交链上
	CreateSettlementBatch(ctx context.Context, tradeIDs []string) (*model.SettlementBatch, error)

	// ConfirmSettlement 确认结算完成
	// 链上交易确认后调用
	ConfirmSettlement(ctx context.Context, batchID, txHash string) error

	// FailSettlement 结算失败处理
	// 链上交易失败后调用，需要回滚
	FailSettlement(ctx context.Context, batchID, reason string) error

	// RollbackTrades 回滚成交
	// 结算失败时回滚余额
	RollbackTrades(ctx context.Context, tradeIDs []string) error
}

// tradeService 成交服务实现
type tradeService struct {
	tradeRepo    repository.TradeRepository
	balanceRepo  repository.BalanceRepository
	balanceCache cache.BalanceRedisRepository // Redis 实时资金回滚
	idGenerator  IDGenerator
	marketConfig MarketConfigProvider
}

// NewTradeService 创建成交服务
func NewTradeService(
	tradeRepo repository.TradeRepository,
	balanceRepo repository.BalanceRepository,
	balanceCache cache.BalanceRedisRepository,
	idGenerator IDGenerator,
	marketConfig MarketConfigProvider,
) TradeService {
	return &tradeService{
		tradeRepo:    tradeRepo,
		balanceRepo:  balanceRepo,
		balanceCache: balanceCache,
		idGenerator:  idGenerator,
		marketConfig: marketConfig,
	}
}

// GetTrade 获取成交详情
func (s *tradeService) GetTrade(ctx context.Context, tradeID string) (*model.Trade, error) {
	return s.tradeRepo.GetByTradeID(ctx, tradeID)
}

// ListTradesByOrder 获取订单相关成交
func (s *tradeService) ListTradesByOrder(ctx context.Context, orderID string) ([]*model.Trade, error) {
	return s.tradeRepo.ListByOrderID(ctx, orderID)
}

// ListTrades 获取用户成交列表
func (s *tradeService) ListTrades(ctx context.Context, wallet string, filter *repository.TradeFilter, page *repository.Pagination) ([]*model.Trade, error) {
	return s.tradeRepo.ListByWallet(ctx, wallet, filter, page)
}

// ListRecentTrades 获取市场最近成交
func (s *tradeService) ListRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	return s.tradeRepo.GetRecentTrades(ctx, market, limit)
}

// CreateSettlementBatch 创建结算批次
func (s *tradeService) CreateSettlementBatch(ctx context.Context, tradeIDs []string) (*model.SettlementBatch, error) {
	if len(tradeIDs) == 0 {
		return nil, errors.New("no trades to settle")
	}

	// 生成批次 ID
	batchIDInt, err := s.idGenerator.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate batch id failed: %w", err)
	}
	batchID := fmt.Sprintf("B%d", batchIDInt)

	// 统计批次信息
	var totalAmount decimal.Decimal
	for _, tradeID := range tradeIDs {
		trade, err := s.tradeRepo.GetByTradeID(ctx, tradeID)
		if err != nil {
			return nil, fmt.Errorf("get trade %s failed: %w", tradeID, err)
		}
		totalAmount = totalAmount.Add(trade.QuoteAmount)
	}

	// 创建批次
	batch := &model.SettlementBatch{
		BatchID:     batchID,
		TradeCount:  len(tradeIDs),
		TotalAmount: totalAmount,
		Status:      model.SettlementStatusPending,
	}

	// 在事务中: 创建批次 + 更新成交状态
	err = s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		if err := s.tradeRepo.CreateBatch(txCtx, batch); err != nil {
			return fmt.Errorf("create batch failed: %w", err)
		}

		for _, tradeID := range tradeIDs {
			if err := s.tradeRepo.UpdateSettlementStatus(txCtx, tradeID,
				model.SettlementStatusMatchedOffchain,
				model.SettlementStatusPending,
				batchID); err != nil {
				return fmt.Errorf("update trade %s status failed: %w", tradeID, err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return batch, nil
}

// ConfirmSettlement 确认结算完成
func (s *tradeService) ConfirmSettlement(ctx context.Context, batchID, txHash string) error {
	// 获取批次
	batch, err := s.tradeRepo.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("get batch failed: %w", err)
	}

	// 获取批次内的成交
	trades, err := s.tradeRepo.ListByBatchID(ctx, batchID)
	if err != nil {
		return fmt.Errorf("list trades by batch failed: %w", err)
	}

	// 收集 trade IDs
	tradeIDs := make([]string, len(trades))
	for i, trade := range trades {
		tradeIDs[i] = trade.TradeID
	}

	now := time.Now().UnixMilli()

	// 在事务中: 更新批次状态 + 更新成交状态 + 结算余额
	err = s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 更新批次状态
		batch.Status = model.SettlementStatusSettledOnchain
		batch.TxHash = txHash
		batch.ConfirmedAt = now
		if err := s.tradeRepo.UpdateBatch(txCtx, batch); err != nil {
			return fmt.Errorf("update batch failed: %w", err)
		}

		// 批量更新成交状态
		if err := s.tradeRepo.BatchUpdateSettlementStatus(txCtx, tradeIDs, model.SettlementStatusSettledOnchain, txHash); err != nil {
			return fmt.Errorf("update trades status failed: %w", err)
		}

		// 结算余额: pending → settled
		for _, trade := range trades {
			cfg, err := s.marketConfig.GetMarket(trade.Market)
			if err != nil {
				continue
			}

			// 结算 Maker 的收益
			if trade.MakerSide == model.OrderSideBuy {
				// Maker 买入，收到 Base Token
				_ = s.balanceRepo.Settle(txCtx, trade.MakerWallet, cfg.BaseToken, trade.Amount)
				_ = s.balanceRepo.Settle(txCtx, trade.TakerWallet, cfg.QuoteToken, trade.QuoteAmount)
			} else {
				// Maker 卖出，收到 Quote Token
				_ = s.balanceRepo.Settle(txCtx, trade.MakerWallet, cfg.QuoteToken, trade.QuoteAmount)
				_ = s.balanceRepo.Settle(txCtx, trade.TakerWallet, cfg.BaseToken, trade.Amount)
			}
		}

		return nil
	})

	return err
}

// FailSettlement 结算失败处理
func (s *tradeService) FailSettlement(ctx context.Context, batchID, reason string) error {
	batch, err := s.tradeRepo.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("get batch failed: %w", err)
	}

	batch.Status = model.SettlementStatusFailed
	batch.LastError = reason
	batch.RetryCount++

	if err := s.tradeRepo.UpdateBatch(ctx, batch); err != nil {
		return fmt.Errorf("update batch failed: %w", err)
	}

	// 获取批次内的成交
	trades, err := s.tradeRepo.ListByBatchID(ctx, batchID)
	if err != nil {
		return fmt.Errorf("list trades by batch failed: %w", err)
	}

	// 收集 trade IDs
	tradeIDs := make([]string, len(trades))
	for i, trade := range trades {
		tradeIDs[i] = trade.TradeID
	}

	// 更新成交状态为失败
	if err := s.tradeRepo.BatchUpdateSettlementStatus(ctx, tradeIDs, model.SettlementStatusFailed, ""); err != nil {
		return fmt.Errorf("update trades status failed: %w", err)
	}

	return nil
}

// RollbackTrades 回滚成交
// 结算失败时调用，将 Redis 中的资金变动反向操作
func (s *tradeService) RollbackTrades(ctx context.Context, tradeIDs []string) error {
	var rollbackErrors []error

	for _, tradeID := range tradeIDs {
		trade, err := s.tradeRepo.GetByTradeID(ctx, tradeID)
		if err != nil {
			logger.Error("rollback: get trade failed",
				zap.String("trade_id", tradeID),
				zap.Error(err))
			rollbackErrors = append(rollbackErrors, err)
			continue
		}

		cfg, err := s.marketConfig.GetMarket(trade.Market)
		if err != nil {
			logger.Error("rollback: get market config failed",
				zap.String("trade_id", tradeID),
				zap.String("market", trade.Market),
				zap.Error(err))
			rollbackErrors = append(rollbackErrors, err)
			continue
		}

		// 1. Redis 原子回滚 (关键步骤: 恢复实时资金状态)
		makerIsBuy := trade.MakerSide == model.OrderSideBuy
		rollbackReq := &cache.RollbackTradeRequest{
			TradeID:     tradeID,
			MakerWallet: trade.MakerWallet,
			TakerWallet: trade.TakerWallet,
			BaseToken:   cfg.BaseToken,
			QuoteToken:  cfg.QuoteToken,
			Amount:      trade.Amount,
			QuoteAmount: trade.QuoteAmount,
			MakerFee:    trade.MakerFee,
			TakerFee:    trade.TakerFee,
			MakerIsBuy:  makerIsBuy,
		}

		if err := s.balanceCache.RollbackTrade(ctx, rollbackReq); err != nil {
			logger.Error("rollback: redis rollback failed",
				zap.String("trade_id", tradeID),
				zap.Error(err))
			rollbackErrors = append(rollbackErrors, err)
			// 继续处理其他成交，不中断
		} else {
			logger.Info("rollback: redis rollback success",
				zap.String("trade_id", tradeID))
		}

		// 2. 更新 DB 成交状态为已回滚
		if err := s.tradeRepo.UpdateSettlementStatus(ctx, tradeID,
			trade.SettlementStatus,
			model.SettlementStatusRolledBack,
			""); err != nil {
			logger.Error("rollback: update trade status failed",
				zap.String("trade_id", tradeID),
				zap.Error(err))
			rollbackErrors = append(rollbackErrors, err)
			continue
		}

		// 3. 创建回滚流水 (Maker 和 Taker 各一条)
		_ = s.balanceRepo.CreateBalanceLog(ctx, &model.BalanceLog{
			Wallet:  trade.MakerWallet,
			Token:   cfg.BaseToken,
			Type:    model.BalanceLogTypeRollback,
			TradeID: tradeID,
			Remark:  fmt.Sprintf("Trade rollback (maker): %s", tradeID),
		})
		_ = s.balanceRepo.CreateBalanceLog(ctx, &model.BalanceLog{
			Wallet:  trade.TakerWallet,
			Token:   cfg.QuoteToken,
			Type:    model.BalanceLogTypeRollback,
			TradeID: tradeID,
			Remark:  fmt.Sprintf("Trade rollback (taker): %s", tradeID),
		})
	}

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback completed with %d errors", len(rollbackErrors))
	}

	return nil
}
