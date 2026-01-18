package repository

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// KlineRepository K线数据仓储接口
type KlineRepository interface {
	// Upsert 更新或插入 K 线
	Upsert(ctx context.Context, kline *model.Kline) error

	// BatchUpsert 批量更新或插入 K 线
	BatchUpsert(ctx context.Context, klines []*model.Kline) error

	// Query 查询 K 线
	Query(ctx context.Context, market string, interval model.KlineInterval,
		startTime, endTime int64, limit int) ([]*model.Kline, error)

	// GetLatest 获取最新的 K 线
	GetLatest(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error)
}

// MarketRepository 交易对仓储接口
type MarketRepository interface {
	// ListActiveMarkets 获取所有活跃的交易对
	ListActiveMarkets(ctx context.Context) ([]*model.Market, error)

	// GetBySymbol 根据符号获取交易对
	GetBySymbol(ctx context.Context, symbol string) (*model.Market, error)

	// Create 创建交易对
	Create(ctx context.Context, market *model.Market) error

	// Update 更新交易对
	Update(ctx context.Context, market *model.Market) error
}

// TradeRepository 成交记录仓储接口
type TradeRepository interface {
	// Create 创建成交记录
	Create(ctx context.Context, trade *model.Trade) error

	// BatchCreate 批量创建成交记录
	BatchCreate(ctx context.Context, trades []*model.Trade) error

	// ListRecent 获取最近的成交记录
	ListRecent(ctx context.Context, market string, limit int) ([]*model.Trade, error)

	// GetByID 根据 ID 获取成交记录
	GetByID(ctx context.Context, tradeID string) (*model.Trade, error)
}
