package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 基础仓储
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建基础仓储
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// txKey 事务上下文键
type txKey struct{}

// DB 返回数据库连接
func (r *Repository) DB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return r.db.WithContext(ctx)
}

// Transaction 执行事务
func (r *Repository) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey{}, tx)
		return fn(txCtx)
	})
}

// TransactionWithRetry 带重试的事务执行
func (r *Repository) TransactionWithRetry(ctx context.Context, maxRetries int, fn func(ctx context.Context) error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = r.Transaction(ctx, fn)
		if err == nil {
			return nil
		}
		// 仅对可重试错误进行重试
		if !isRetryableError(err) {
			return err
		}
		// 指数退避
		time.Sleep(time.Duration(1<<uint(i)) * 100 * time.Millisecond)
	}
	return err
}

// isRetryableError 判断是否为可重试错误
func isRetryableError(err error) bool {
	// TODO: 根据具体数据库错误码判断
	return false
}

// Pagination 分页参数
type Pagination struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
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
	ForUpdate bool
	NoWait    bool
}

// ApplyLock 应用锁选项
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

// TimeRange 时间范围
type TimeRange struct {
	Start int64
	End   int64
}

// IsValid 检查时间范围是否有效
func (tr *TimeRange) IsValid() bool {
	return tr != nil && tr.Start > 0 && tr.End > 0 && tr.Start <= tr.End
}
