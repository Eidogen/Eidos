package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// ClearTrade 清算成交 (原子操作)
func (r *balanceRedisRepository) ClearTrade(ctx context.Context, req *ClearTradeRequest) error {
	startTime := time.Now()
	defer func() {
		metrics.RedisOperationLatency.WithLabelValues("clear_trade").Observe(time.Since(startTime).Seconds())
	}()

	tradeKey := tradeIdempotentKey(req.TradeID)
	makerBaseKey := balanceKey(req.MakerWallet, req.BaseToken)
	makerQuoteKey := balanceKey(req.MakerWallet, req.QuoteToken)
	takerBaseKey := balanceKey(req.TakerWallet, req.BaseToken)
	takerQuoteKey := balanceKey(req.TakerWallet, req.QuoteToken)
	makerOrderFreezeKey := orderFreezeKey(req.MakerOrderID)
	takerOrderFreezeKey := orderFreezeKey(req.TakerOrderID)
	feeBucketKeyStr := feeBucketKey(req.Market, req.FeeToken, req.FeeBucketID)
	now := time.Now().UnixMilli()

	makerIsBuyFlag := "0"
	if req.MakerIsBuy {
		makerIsBuyFlag = "1"
	}

	result, err := luaClearTrade.Run(ctx, r.rdb,
		[]string{tradeKey, makerBaseKey, makerQuoteKey, takerBaseKey, takerQuoteKey,
			makerOrderFreezeKey, takerOrderFreezeKey, feeBucketKeyStr},
		req.Amount.String(), req.QuoteAmount.String(), req.MakerFee.String(), req.TakerFee.String(),
		makerIsBuyFlag, now,
	).Result()

	if err != nil {
		return fmt.Errorf("clear trade failed: %w", err)
	}

	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) >= 2 {
		if resultSlice[0] == "err" {
			errStr := resultSlice[1].(string)
			if errStr == "trade_already_processed" {
				return ErrRedisTradeProcessed
			}
			return errors.New(errStr)
		}
	}

	return nil
}

// RollbackTrade 回滚成交 (结算失败时使用)
// 将成交的资金变动反向操作:
// - Maker 收益从 pending_available 扣回
// - Taker 收益从 pending_available 扣回
// - 双方原来扣除的冻结退回 pending_frozen (因为已经从 frozen 扣除，无法区分是 settled 还是 pending)
func (r *balanceRedisRepository) RollbackTrade(ctx context.Context, req *RollbackTradeRequest) error {
	rollbackKey := fmt.Sprintf("eidos:trading:rollback:%s", req.TradeID)
	makerBaseKey := balanceKey(req.MakerWallet, req.BaseToken)
	makerQuoteKey := balanceKey(req.MakerWallet, req.QuoteToken)
	takerBaseKey := balanceKey(req.TakerWallet, req.BaseToken)
	takerQuoteKey := balanceKey(req.TakerWallet, req.QuoteToken)
	now := time.Now().UnixMilli()

	makerIsBuyFlag := "0"
	if req.MakerIsBuy {
		makerIsBuyFlag = "1"
	}

	_, err := luaRollbackTrade.Run(ctx, r.rdb,
		[]string{rollbackKey, makerBaseKey, makerQuoteKey, takerBaseKey, takerQuoteKey},
		req.Amount.String(), req.QuoteAmount.String(), req.MakerFee.String(), req.TakerFee.String(),
		makerIsBuyFlag, now,
	).Result()

	if err != nil {
		return fmt.Errorf("rollback trade failed: %w", err)
	}

	return nil
}

// CreditFeeBucket 增加手续费分桶
func (r *balanceRedisRepository) CreditFeeBucket(ctx context.Context, market, token string, bucketID int, amount decimal.Decimal) error {
	key := feeBucketKey(market, token, bucketID)
	return r.rdb.IncrByFloat(ctx, key, amount.InexactFloat64()).Err()
}

// CheckTradeProcessed 检查成交是否已处理
func (r *balanceRedisRepository) CheckTradeProcessed(ctx context.Context, tradeID string) (bool, error) {
	key := tradeIdempotentKey(tradeID)
	result, err := r.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("check trade processed failed: %w", err)
	}
	return result > 0, nil
}

// MarkTradeProcessed 标记成交已处理
func (r *balanceRedisRepository) MarkTradeProcessed(ctx context.Context, tradeID string, ttl time.Duration) error {
	key := tradeIdempotentKey(tradeID)
	return r.rdb.SetEx(ctx, key, "1", ttl).Err()
}
