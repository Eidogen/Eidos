package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderAlreadyExists = errors.New("order already exists")
	ErrOptimisticLock     = errors.New("optimistic lock conflict")
)

// OrderRepository 订单仓储接口
type OrderRepository interface {
	// Create 创建订单
	Create(ctx context.Context, order *model.Order) error

	// GetByID 根据 ID 查询订单
	GetByID(ctx context.Context, id int64) (*model.Order, error)

	// GetByOrderID 根据订单 ID 查询
	GetByOrderID(ctx context.Context, orderID string) (*model.Order, error)

	// GetByClientOrderID 根据客户端订单 ID 查询 (幂等键)
	GetByClientOrderID(ctx context.Context, wallet, clientOrderID string) (*model.Order, error)

	// GetByWalletNonce 根据钱包和 Nonce 查询 (幂等键)
	GetByWalletNonce(ctx context.Context, wallet string, nonce uint64) (*model.Order, error)

	// ListByWallet 查询用户订单列表
	ListByWallet(ctx context.Context, wallet string, filter *OrderFilter, page *Pagination) ([]*model.Order, error)

	// ListByMarket 查询市场订单列表
	ListByMarket(ctx context.Context, market string, filter *OrderFilter, page *Pagination) ([]*model.Order, error)

	// ListOpenOrders 查询活跃订单 (OPEN + PARTIAL)
	ListOpenOrders(ctx context.Context, wallet string, market string) ([]*model.Order, error)

	// ListExpiredOrders 查询已过期的订单
	// expireBefore: 过期时间阈值 (毫秒)
	// status: 只查询指定状态的订单
	// limit: 返回数量上限
	ListExpiredOrders(ctx context.Context, expireBefore int64, status model.OrderStatus, limit int) ([]*model.Order, error)

	// Update 更新订单
	Update(ctx context.Context, order *model.Order) error

	// UpdateStatus 更新订单状态
	UpdateStatus(ctx context.Context, orderID string, oldStatus, newStatus model.OrderStatus) error

	// UpdateFilled 更新成交信息 (原子操作)
	UpdateFilled(ctx context.Context, orderID string, filledAmount, filledQuote string, newStatus model.OrderStatus) error

	// BatchCreate 批量创建订单
	BatchCreate(ctx context.Context, orders []*model.Order) error

	// CountByWallet 统计用户订单数
	CountByWallet(ctx context.Context, wallet string, filter *OrderFilter) (int64, error)
}

// OrderFilter 订单查询过滤条件
type OrderFilter struct {
	Market    string              // 交易对
	Side      *model.OrderSide    // 买卖方向
	Type      *model.OrderType    // 订单类型
	Statuses  []model.OrderStatus // 订单状态列表
	TimeRange *TimeRange          // 时间范围
}

// orderRepository 订单仓储实现
type orderRepository struct {
	*Repository
}

// NewOrderRepository 创建订单仓储
func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{
		Repository: NewRepository(db),
	}
}

// Create 创建订单
func (r *orderRepository) Create(ctx context.Context, order *model.Order) error {
	result := r.DB(ctx).Create(order)
	if result.Error != nil {
		// 检查唯一约束冲突
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return ErrOrderAlreadyExists
		}
		return fmt.Errorf("create order failed: %w", result.Error)
	}
	return nil
}

// GetByID 根据 ID 查询订单
func (r *orderRepository) GetByID(ctx context.Context, id int64) (*model.Order, error) {
	var order model.Order
	result := r.DB(ctx).Where("id = ?", id).First(&order)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order by id failed: %w", result.Error)
	}
	return &order, nil
}

// GetByOrderID 根据订单 ID 查询
func (r *orderRepository) GetByOrderID(ctx context.Context, orderID string) (*model.Order, error) {
	var order model.Order
	result := r.DB(ctx).Where("order_id = ?", orderID).First(&order)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order by order_id failed: %w", result.Error)
	}
	return &order, nil
}

// GetByClientOrderID 根据客户端订单 ID 查询 (幂等键)
func (r *orderRepository) GetByClientOrderID(ctx context.Context, wallet, clientOrderID string) (*model.Order, error) {
	var order model.Order
	result := r.DB(ctx).Where("wallet = ? AND client_order_id = ?", wallet, clientOrderID).First(&order)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order by client_order_id failed: %w", result.Error)
	}
	return &order, nil
}

// GetByWalletNonce 根据钱包和 Nonce 查询 (幂等键)
func (r *orderRepository) GetByWalletNonce(ctx context.Context, wallet string, nonce uint64) (*model.Order, error) {
	var order model.Order
	result := r.DB(ctx).Where("wallet = ? AND nonce = ?", wallet, nonce).First(&order)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order by wallet_nonce failed: %w", result.Error)
	}
	return &order, nil
}

// ListByWallet 查询用户订单列表
func (r *orderRepository) ListByWallet(ctx context.Context, wallet string, filter *OrderFilter, page *Pagination) ([]*model.Order, error) {
	db := r.DB(ctx).Where("wallet = ?", wallet)
	db = r.applyFilter(db, filter)

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.Order{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count orders failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var orders []*model.Order
	db = db.Order("created_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("list orders by wallet failed: %w", err)
	}
	return orders, nil
}

// ListByMarket 查询市场订单列表
func (r *orderRepository) ListByMarket(ctx context.Context, market string, filter *OrderFilter, page *Pagination) ([]*model.Order, error) {
	db := r.DB(ctx).Where("market = ?", market)
	db = r.applyFilter(db, filter)

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.Order{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count orders failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var orders []*model.Order
	db = db.Order("created_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("list orders by market failed: %w", err)
	}
	return orders, nil
}

// ListOpenOrders 查询活跃订单 (OPEN + PARTIAL)
func (r *orderRepository) ListOpenOrders(ctx context.Context, wallet string, market string) ([]*model.Order, error) {
	db := r.DB(ctx).Where("status IN ?", []model.OrderStatus{model.OrderStatusOpen, model.OrderStatusPartial})

	if wallet != "" {
		db = db.Where("wallet = ?", wallet)
	}
	if market != "" {
		db = db.Where("market = ?", market)
	}

	var orders []*model.Order
	if err := db.Order("created_at ASC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("list open orders failed: %w", err)
	}
	return orders, nil
}

// ListExpiredOrders 查询已过期的订单
func (r *orderRepository) ListExpiredOrders(ctx context.Context, expireBefore int64, status model.OrderStatus, limit int) ([]*model.Order, error) {
	var orders []*model.Order
	result := r.DB(ctx).
		Where("expire_at < ? AND status = ?", expireBefore, status).
		Order("expire_at ASC").
		Limit(limit).
		Find(&orders)

	if result.Error != nil {
		return nil, fmt.Errorf("list expired orders failed: %w", result.Error)
	}
	return orders, nil
}

// Update 更新订单
func (r *orderRepository) Update(ctx context.Context, order *model.Order) error {
	result := r.DB(ctx).Save(order)
	if result.Error != nil {
		return fmt.Errorf("update order failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// UpdateStatus 更新订单状态
func (r *orderRepository) UpdateStatus(ctx context.Context, orderID string, oldStatus, newStatus model.OrderStatus) error {
	result := r.DB(ctx).Model(&model.Order{}).
		Where("order_id = ? AND status = ?", orderID, oldStatus).
		Update("status", newStatus)

	if result.Error != nil {
		return fmt.Errorf("update order status failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// UpdateFilled 更新成交信息 (原子操作)
func (r *orderRepository) UpdateFilled(ctx context.Context, orderID string, filledAmount, filledQuote string, newStatus model.OrderStatus) error {
	// 使用原生 SQL 进行原子更新
	sql := `UPDATE trading_orders
			SET filled_amount = filled_amount + ?,
				filled_quote = filled_quote + ?,
				status = ?,
				updated_at = ?
			WHERE order_id = ?`

	result := r.DB(ctx).Exec(sql, filledAmount, filledQuote, newStatus, nowMilli(), orderID)
	if result.Error != nil {
		return fmt.Errorf("update filled failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// BatchCreate 批量创建订单
func (r *orderRepository) BatchCreate(ctx context.Context, orders []*model.Order) error {
	if len(orders) == 0 {
		return nil
	}

	// 使用 ON CONFLICT DO NOTHING 忽略重复
	result := r.DB(ctx).Clauses(clause.OnConflict{
		DoNothing: true,
	}).CreateInBatches(orders, 100)

	if result.Error != nil {
		return fmt.Errorf("batch create orders failed: %w", result.Error)
	}
	return nil
}

// CountByWallet 统计用户订单数
func (r *orderRepository) CountByWallet(ctx context.Context, wallet string, filter *OrderFilter) (int64, error) {
	db := r.DB(ctx).Model(&model.Order{}).Where("wallet = ?", wallet)
	db = r.applyFilter(db, filter)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count orders failed: %w", err)
	}
	return count, nil
}

// applyFilter 应用过滤条件
func (r *orderRepository) applyFilter(db *gorm.DB, filter *OrderFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	if filter.Market != "" {
		db = db.Where("market = ?", filter.Market)
	}
	if filter.Side != nil {
		db = db.Where("side = ?", *filter.Side)
	}
	if filter.Type != nil {
		db = db.Where("type = ?", *filter.Type)
	}
	if len(filter.Statuses) > 0 {
		db = db.Where("status IN ?", filter.Statuses)
	}
	if filter.TimeRange != nil && filter.TimeRange.IsValid() {
		db = db.Where("created_at >= ? AND created_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
	}

	return db
}

// nowMilli 返回当前毫秒时间戳
func nowMilli() int64 {
	return time.Now().UnixMilli()
}
