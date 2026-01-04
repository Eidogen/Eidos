package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 基础仓储接口
// 所有仓储实现都应该嵌入此结构
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建基础仓储
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// DB 返回数据库连接
// 如果 context 中有事务，返回事务连接
func (r *Repository) DB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return r.db.WithContext(ctx)
}

// txKey 事务上下文键
type txKey struct{}

// Transaction 执行事务
// fn 中的所有数据库操作都在同一事务中执行
func (r *Repository) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey{}, tx)
		return fn(txCtx)
	})
}

// TransactionWithOptions 带选项的事务执行
func (r *Repository) TransactionWithOptions(ctx context.Context, opts *TxOptions, fn func(ctx context.Context) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey{}, tx)
		return fn(txCtx)
	})
}

// TxOptions 事务选项
type TxOptions struct {
	Timeout    time.Duration // 事务超时时间
	RetryCount int           // 重试次数 (乐观锁冲突时)
}

// DefaultTxOptions 默认事务选项
var DefaultTxOptions = &TxOptions{
	Timeout:    5 * time.Second,
	RetryCount: 3,
}

// Pagination 分页参数
type Pagination struct {
	Page     int   // 页码 (从 1 开始)
	PageSize int   // 每页数量
	Total    int64 // 总数 (查询后填充)
}

// Offset 计算偏移量
func (p *Pagination) Offset() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PageSize
}

// Limit 返回限制数量
func (p *Pagination) Limit() int {
	if p.PageSize <= 0 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p.PageSize
}

// QueryOptions 查询选项
type QueryOptions struct {
	ForUpdate bool // SELECT FOR UPDATE 锁定
	NoWait    bool // NOWAIT 不等待锁
}

// ApplyLock 应用锁选项到查询
func (o *QueryOptions) ApplyLock(db *gorm.DB) *gorm.DB {
	if o == nil || !o.ForUpdate {
		return db
	}
	if o.NoWait {
		return db.Clauses(clause.Locking{
			Strength: "UPDATE",
			Options:  "NOWAIT",
		})
	}
	return db.Clauses(clause.Locking{
		Strength: "UPDATE",
	})
}

// TimeRange 时间范围查询
type TimeRange struct {
	Start int64 // 开始时间 (毫秒)
	End   int64 // 结束时间 (毫秒)
}

// IsValid 检查时间范围是否有效
func (tr *TimeRange) IsValid() bool {
	return tr != nil && tr.Start > 0 && tr.End > 0 && tr.Start <= tr.End
}
