package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ClearingService 清算服务接口
type ClearingService interface {
	// ProcessTradeResult 处理成交结果
	ProcessTradeResult(ctx context.Context, msg *worker.TradeResultMessage) error
	// HandleSettlementConfirm 处理结算确认
	HandleSettlementConfirm(ctx context.Context, msg *worker.SettlementConfirmedMessage) error
	// Shutdown 优雅关闭，等待所有异步任务完成
	Shutdown(ctx context.Context) error
}

// clearingService 清算服务实现
type clearingService struct {
	db             *gorm.DB
	tradeRepo      repository.TradeRepository
	orderRepo      repository.OrderRepository
	balanceRepo    repository.BalanceRepository
	balanceCache   cache.BalanceRedisRepository // Redis 作为实时资金真相
	marketProvider MarketConfigProvider
	asyncTasks     *AsyncTaskManager // 异步任务管理器
}

// NewClearingService 创建清算服务
func NewClearingService(
	db *gorm.DB,
	tradeRepo repository.TradeRepository,
	orderRepo repository.OrderRepository,
	balanceRepo repository.BalanceRepository,
	balanceCache cache.BalanceRedisRepository,
	marketProvider MarketConfigProvider,
) ClearingService {
	return &clearingService{
		db:             db,
		tradeRepo:      tradeRepo,
		orderRepo:      orderRepo,
		balanceRepo:    balanceRepo,
		balanceCache:   balanceCache,
		marketProvider: marketProvider,
		asyncTasks:     GetAsyncTaskManager(),
	}
}

// ProcessTradeResult 处理成交结果
// 使用 Redis Lua 原子操作清算成交，确保实时资金一致性
// 注意: ClearTrade Lua 脚本内部已包含幂等检查和标记，无需额外调用 CheckTradeProcessed/MarkTradeProcessed
func (s *clearingService) ProcessTradeResult(ctx context.Context, msg *worker.TradeResultMessage) error {
	// 1. 解析金额
	price, err := decimal.NewFromString(msg.Price)
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}
	size, err := decimal.NewFromString(msg.Size)
	if err != nil {
		return fmt.Errorf("invalid size: %w", err)
	}
	quoteAmount, err := decimal.NewFromString(msg.QuoteAmount)
	if err != nil {
		return fmt.Errorf("invalid quote amount: %w", err)
	}
	makerFee, err := decimal.NewFromString(msg.MakerFee)
	if err != nil {
		return fmt.Errorf("invalid maker fee: %w", err)
	}
	takerFee, err := decimal.NewFromString(msg.TakerFee)
	if err != nil {
		return fmt.Errorf("invalid taker fee: %w", err)
	}

	// 2. 获取市场配置
	marketCfg, err := s.marketProvider.GetMarket(msg.Market)
	if err != nil {
		return fmt.Errorf("get market config: %w", err)
	}

	// 3. Redis 原子清算成交 (Lua 脚本内部包含幂等检查: EXISTS trade_key + SETEX trade_key)
	clearReq := &cache.ClearTradeRequest{
		TradeID:      msg.TradeID,
		MakerWallet:  msg.Maker,
		TakerWallet:  msg.Taker,
		MakerOrderID: msg.MakerOrderID,
		TakerOrderID: msg.TakerOrderID,
		BaseToken:    marketCfg.BaseToken,
		QuoteToken:   marketCfg.QuoteToken,
		Price:        price,
		Amount:       size,
		QuoteAmount:  quoteAmount,
		MakerFee:     makerFee,
		TakerFee:     takerFee,
		FeeToken:     marketCfg.QuoteToken, // 手续费用 QuoteToken 计价
		MakerIsBuy:   msg.MakerIsBuyer,
		Market:       msg.Market,
		FeeBucketID:  int(msg.Timestamp / 3600000), // 按小时分桶
	}

	if err := s.balanceCache.ClearTrade(ctx, clearReq); err != nil {
		if errors.Is(err, cache.ErrRedisTradeProcessed) {
			logger.Debug("trade already processed (idempotent)",
				zap.String("trade_id", msg.TradeID))
			return nil
		}
		return fmt.Errorf("redis clear trade: %w", err)
	}

	// 4. 异步写入 DB (Redis 已完成实时清算，DB 用于持久化)
	// 使用 AsyncTaskManager 管理，提供超时控制和错误日志
	s.asyncTasks.Submit("persistTradeToDB", msg.TradeID, func(taskCtx context.Context) error {
		return s.persistTradeToDB(taskCtx, msg, marketCfg, price, size, quoteAmount, makerFee, takerFee)
	})

	logger.Info("trade cleared successfully",
		zap.String("trade_id", msg.TradeID),
		zap.String("market", msg.Market),
	)

	return nil
}

// persistTradeToDB 持久化成交记录到数据库
func (s *clearingService) persistTradeToDB(ctx context.Context, msg *worker.TradeResultMessage, cfg *MarketConfig,
	price, size, quoteAmount, makerFee, takerFee decimal.Decimal) error {

	var makerSide model.OrderSide
	if msg.MakerIsBuyer {
		makerSide = model.OrderSideBuy
	} else {
		makerSide = model.OrderSideSell
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 插入成交记录 (幂等)
		trade := &model.Trade{
			TradeID:          msg.TradeID,
			Market:           msg.Market,
			MakerOrderID:     msg.MakerOrderID,
			TakerOrderID:     msg.TakerOrderID,
			MakerWallet:      msg.Maker,
			TakerWallet:      msg.Taker,
			Price:            price,
			Amount:           size,
			QuoteAmount:      quoteAmount,
			MakerFee:         makerFee,
			TakerFee:         takerFee,
			MakerSide:        makerSide,
			SettlementStatus: model.SettlementStatusMatchedOffchain,
			MatchedAt:        msg.Timestamp,
		}

		result := tx.Exec(`
			INSERT INTO trades (trade_id, market, maker_order_id, taker_order_id, maker_wallet, taker_wallet,
			                    price, amount, quote_amount, maker_fee, taker_fee,
			                    maker_side, settlement_status, matched_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (trade_id) DO NOTHING
		`, trade.TradeID, trade.Market, trade.MakerOrderID, trade.TakerOrderID,
			trade.MakerWallet, trade.TakerWallet, trade.Price.String(),
			trade.Amount.String(), trade.QuoteAmount.String(), trade.MakerFee.String(),
			trade.TakerFee.String(), trade.MakerSide, trade.SettlementStatus, trade.MatchedAt,
			msg.Timestamp, msg.Timestamp)

		if result.Error != nil {
			return fmt.Errorf("insert trade: %w", result.Error)
		}

		// 2. 更新订单状态
		if err := s.updateOrdersFilled(ctx, tx, msg, size); err != nil {
			return fmt.Errorf("update orders filled: %w", err)
		}

		return nil
	})
}

// updateOrdersFilled 更新订单已成交数量
// 注意: 为避免死锁，按订单 ID 排序后更新，确保多实例并发时锁定顺序一致
func (s *clearingService) updateOrdersFilled(ctx context.Context, tx *gorm.DB, msg *worker.TradeResultMessage, size decimal.Decimal) error {
	// 按订单 ID 排序，确保一致的更新顺序，避免死锁
	orderIDs := []string{msg.MakerOrderID, msg.TakerOrderID}
	sort.Strings(orderIDs)

	for _, orderID := range orderIDs {
		if err := s.updateOrderFilled(tx, orderID, size); err != nil {
			return fmt.Errorf("update order %s: %w", orderID, err)
		}
	}

	return nil
}

// updateOrderFilled 更新单个订单的已成交数量
func (s *clearingService) updateOrderFilled(tx *gorm.DB, orderID string, size decimal.Decimal) error {
	now := time.Now().UnixMilli()
	result := tx.Exec(`
		UPDATE orders
		SET filled_amount = filled_amount + ?,
		    status = CASE
		        WHEN filled_amount + ? >= amount THEN ?
		        ELSE ?
		    END,
		    updated_at = ?
		WHERE order_id = ?
	`, size, size, model.OrderStatusFilled, model.OrderStatusPartial, now, orderID)

	return result.Error
}

// HandleSettlementConfirm 处理结算确认
func (s *clearingService) HandleSettlementConfirm(ctx context.Context, msg *worker.SettlementConfirmedMessage) error {
	if msg.Status == "confirmed" {
		// 结算成功: 更新 DB 状态 + Redis 余额结算 (pending → settled)
		return s.handleSettlementSuccess(ctx, msg)
	}

	// 结算失败: 回滚 Redis 余额 + 更新 DB 状态
	return s.handleSettlementFailure(ctx, msg)
}

// handleSettlementSuccess 处理结算成功
func (s *clearingService) handleSettlementSuccess(ctx context.Context, msg *worker.SettlementConfirmedMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 更新成交记录状态
		for _, tradeID := range msg.TradeIDs {
			result := tx.Exec(`
				UPDATE trades
				SET settlement_status = ?,
				    tx_hash = ?,
				    settled_at = ?,
				    updated_at = ?
				WHERE trade_id = ?
			`, model.SettlementStatusSettledOnchain, msg.TxHash, msg.Timestamp, msg.Timestamp, tradeID)

			if result.Error != nil {
				return fmt.Errorf("update trade status: %w", result.Error)
			}
		}

		// 2. Redis 余额结算: pending → settled
		// 异步执行，因为 DB 事务是关键路径
		s.asyncTasks.Submit("settleBalances", msg.SettlementID, func(taskCtx context.Context) error {
			return s.settleBalancesForTrades(taskCtx, msg.TradeIDs)
		})

		logger.Info("settlement confirmed",
			zap.String("settlement_id", msg.SettlementID),
			zap.String("tx_hash", msg.TxHash),
			zap.Int64("block_number", msg.BlockNumber),
			zap.Int("trade_count", len(msg.TradeIDs)),
		)

		return nil
	})
}

// handleSettlementFailure 处理结算失败 (回滚)
func (s *clearingService) handleSettlementFailure(ctx context.Context, msg *worker.SettlementConfirmedMessage) error {
	logger.Warn("settlement failed, starting rollback",
		zap.String("settlement_id", msg.SettlementID),
		zap.String("status", msg.Status),
		zap.Int("trade_count", len(msg.TradeIDs)),
	)

	var rollbackErrors []error

	// 1. 逐笔回滚 Redis 余额 (关键: 恢复实时资金状态)
	for _, tradeID := range msg.TradeIDs {
		if err := s.rollbackSingleTrade(ctx, tradeID); err != nil {
			logger.Error("rollback trade failed",
				zap.String("trade_id", tradeID),
				zap.Error(err))
			rollbackErrors = append(rollbackErrors, err)
			// 继续处理其他成交
		}
	}

	// 2. 更新 DB 成交状态
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		for _, tradeID := range msg.TradeIDs {
			result := tx.Exec(`
				UPDATE trades
				SET settlement_status = ?,
				    updated_at = ?
				WHERE trade_id = ?
			`, model.SettlementStatusRolledBack, now, tradeID)

			if result.Error != nil {
				return fmt.Errorf("update trade status: %w", result.Error)
			}
		}
		return nil
	}); err != nil {
		logger.Error("update trade status to rolled_back failed",
			zap.String("settlement_id", msg.SettlementID),
			zap.Error(err))
		rollbackErrors = append(rollbackErrors, err)
	}

	if len(rollbackErrors) > 0 {
		logger.Error("settlement rollback completed with errors",
			zap.String("settlement_id", msg.SettlementID),
			zap.Int("error_count", len(rollbackErrors)))
		return fmt.Errorf("rollback completed with %d errors", len(rollbackErrors))
	}

	logger.Info("settlement rollback completed successfully",
		zap.String("settlement_id", msg.SettlementID),
		zap.Int("trade_count", len(msg.TradeIDs)))

	return nil
}

// rollbackSingleTrade 回滚单笔成交的 Redis 余额
func (s *clearingService) rollbackSingleTrade(ctx context.Context, tradeID string) error {
	// 从 DB 获取成交信息
	var trade model.Trade
	if err := s.db.WithContext(ctx).Where("trade_id = ?", tradeID).First(&trade).Error; err != nil {
		return fmt.Errorf("get trade: %w", err)
	}

	// 获取市场配置
	marketCfg, err := s.marketProvider.GetMarket(trade.Market)
	if err != nil {
		return fmt.Errorf("get market config: %w", err)
	}

	// 执行 Redis 回滚
	rollbackReq := &cache.RollbackTradeRequest{
		TradeID:     tradeID,
		MakerWallet: trade.MakerWallet,
		TakerWallet: trade.TakerWallet,
		BaseToken:   marketCfg.BaseToken,
		QuoteToken:  marketCfg.QuoteToken,
		Amount:      trade.Amount,
		QuoteAmount: trade.QuoteAmount,
		MakerFee:    trade.MakerFee,
		TakerFee:    trade.TakerFee,
		MakerIsBuy:  trade.MakerSide == model.OrderSideBuy,
	}

	if err := s.balanceCache.RollbackTrade(ctx, rollbackReq); err != nil {
		return fmt.Errorf("redis rollback: %w", err)
	}

	logger.Info("trade rollback success",
		zap.String("trade_id", tradeID),
		zap.String("maker", trade.MakerWallet),
		zap.String("taker", trade.TakerWallet))

	return nil
}

// settleBalancesForTrades 结算成交涉及的余额 (pending → settled)
func (s *clearingService) settleBalancesForTrades(ctx context.Context, tradeIDs []string) error {
	for _, tradeID := range tradeIDs {
		var trade model.Trade
		if err := s.db.WithContext(ctx).Where("trade_id = ?", tradeID).First(&trade).Error; err != nil {
			logger.Error("settle: get trade failed",
				zap.String("trade_id", tradeID),
				zap.Error(err))
			continue
		}

		marketCfg, err := s.marketProvider.GetMarket(trade.Market)
		if err != nil {
			logger.Error("settle: get market config failed",
				zap.String("trade_id", tradeID),
				zap.Error(err))
			continue
		}

		// 根据 Maker 方向结算双方的 pending → settled
		if trade.MakerSide == model.OrderSideBuy {
			// Maker 买入: Maker 收到 base, Taker 收到 quote
			makerReceive := trade.Amount.Sub(trade.MakerFee)
			takerReceive := trade.QuoteAmount.Sub(trade.TakerFee)

			_ = s.balanceCache.Settle(ctx, trade.MakerWallet, marketCfg.BaseToken, makerReceive)
			_ = s.balanceCache.Settle(ctx, trade.TakerWallet, marketCfg.QuoteToken, takerReceive)
		} else {
			// Maker 卖出: Maker 收到 quote, Taker 收到 base
			makerReceive := trade.QuoteAmount.Sub(trade.MakerFee)
			takerReceive := trade.Amount.Sub(trade.TakerFee)

			_ = s.balanceCache.Settle(ctx, trade.MakerWallet, marketCfg.QuoteToken, makerReceive)
			_ = s.balanceCache.Settle(ctx, trade.TakerWallet, marketCfg.BaseToken, takerReceive)
		}

		logger.Debug("trade settled",
			zap.String("trade_id", tradeID))
	}

	return nil
}

// Shutdown 优雅关闭，等待所有异步 DB 写入任务完成
func (s *clearingService) Shutdown(ctx context.Context) error {
	// 使用全局 AsyncTaskManager 的 Shutdown
	// 注意: 实际关闭由 app 层统一调用 GetAsyncTaskManager().Shutdown()
	logger.Info("clearing service shutdown completed")
	return nil
}
