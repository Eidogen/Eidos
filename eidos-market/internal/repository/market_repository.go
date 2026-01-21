package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// marketRepository 交易对仓储实现
// [eidos-admin] 交易对配置管理
//   - eidos-admin 需要提供交易对的 CRUD 管理界面
//   - 新增/修改交易对后，需要通知 eidos-market 重新加载
//   - 方式1: 通过 Kafka 发送 market-config-update 事件
//   - 方式2: eidos-market 定时轮询 (当前实现，启动时加载一次)
//   - 方式3: 提供 gRPC 接口供 eidos-admin 调用触发重新加载
type marketRepository struct {
	db *gorm.DB
}

// NewMarketRepository 创建交易对仓储
func NewMarketRepository(db *gorm.DB) MarketRepository {
	return &marketRepository{db: db}
}

// ListActiveMarkets 获取所有活跃的交易对
func (r *marketRepository) ListActiveMarkets(ctx context.Context) ([]*model.Market, error) {
	var markets []*model.Market
	err := r.db.WithContext(ctx).
		Where("status = ?", model.MarketStatusActive).
		Order("symbol ASC").
		Find(&markets).Error

	if err != nil {
		return nil, err
	}

	return markets, nil
}

// GetBySymbol 根据符号获取交易对
func (r *marketRepository) GetBySymbol(ctx context.Context, symbol string) (*model.Market, error) {
	var market model.Market
	err := r.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		First(&market).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &market, nil
}

// Create 创建交易对
func (r *marketRepository) Create(ctx context.Context, market *model.Market) error {
	return r.db.WithContext(ctx).Create(market).Error
}

// Update 更新交易对
func (r *marketRepository) Update(ctx context.Context, market *model.Market) error {
	return r.db.WithContext(ctx).Save(market).Error
}

// ListAllMarkets 获取所有交易对（包括未激活的）
func (r *marketRepository) ListAllMarkets(ctx context.Context) ([]*model.Market, error) {
	var markets []*model.Market
	err := r.db.WithContext(ctx).
		Order("symbol ASC").
		Find(&markets).Error

	if err != nil {
		return nil, err
	}

	return markets, nil
}
