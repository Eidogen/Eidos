package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostgreSQL 可重试错误码
// 参考: https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	// Class 40 — Transaction Rollback
	pgErrSerializationFailure = "40001" // serialization_failure
	pgErrDeadlockDetected     = "40P01" // deadlock_detected

	// Class 08 — Connection Exception
	pgErrConnectionFailure    = "08006" // connection_failure
	pgErrConnectionException  = "08000" // connection_exception
	pgErrSQLClientCantConnect = "08001" // sqlclient_unable_to_establish_sqlconnection

	// Class 53 — Insufficient Resources
	pgErrInsufficientResources = "53000" // insufficient_resources
	pgErrDiskFull              = "53100" // disk_full
	pgErrOutOfMemory           = "53200" // out_of_memory
	pgErrTooManyConnections    = "53300" // too_many_connections

	// Class 57 — Operator Intervention
	pgErrOperatorIntervention = "57000" // operator_intervention
	pgErrQueryCanceled        = "57014" // query_canceled
	pgErrAdminShutdown        = "57P01" // admin_shutdown
	pgErrCrashShutdown        = "57P02" // crash_shutdown
	pgErrCannotConnectNow     = "57P03" // cannot_connect_now
	pgErrDatabaseDropped      = "57P04" // database_dropped
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
// 主要包括: 死锁、序列化失败、连接问题、资源不足等临时性错误
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否为 PostgreSQL 错误
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		// 事务回滚类错误 - 可重试
		case pgErrSerializationFailure, pgErrDeadlockDetected:
			return true
		// 连接异常类错误 - 可重试
		case pgErrConnectionFailure, pgErrConnectionException, pgErrSQLClientCantConnect:
			return true
		// 资源不足类错误 - 可重试 (可能是临时性的)
		case pgErrInsufficientResources, pgErrTooManyConnections:
			return true
		// 操作干预类错误 - 部分可重试
		case pgErrQueryCanceled, pgErrCannotConnectNow:
			return true
		// 磁盘满、内存不足 - 不重试 (需要人工干预)
		case pgErrDiskFull, pgErrOutOfMemory:
			return false
		// 管理员关闭、崩溃、数据库删除 - 不重试
		case pgErrAdminShutdown, pgErrCrashShutdown, pgErrDatabaseDropped:
			return false
		}
	}

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
