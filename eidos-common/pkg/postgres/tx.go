package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// TxOptions 事务选项
type TxOptions struct {
	Isolation sql.IsolationLevel
	ReadOnly  bool
	Timeout   time.Duration
}

// DefaultTxOptions 默认事务选项
func DefaultTxOptions() *TxOptions {
	return &TxOptions{
		Isolation: sql.LevelDefault,
		ReadOnly:  false,
		Timeout:   30 * time.Second,
	}
}

// Tx 事务封装
type Tx struct {
	tx      *sql.Tx
	pool    *Pool
	start   time.Time
	options *TxOptions
	done    bool
}

// BeginTx 开始事务
func (p *Pool) BeginTx(ctx context.Context, opts *TxOptions) (*Tx, error) {
	if opts == nil {
		opts = DefaultTxOptions()
	}

	// 设置超时
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	sqlOpts := &sql.TxOptions{
		Isolation: opts.Isolation,
		ReadOnly:  opts.ReadOnly,
	}

	tx, err := p.db.BeginTx(ctx, sqlOpts)
	if err != nil {
		logger.Error("failed to begin transaction",
			"error", err,
		)
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	logger.Debug("transaction started",
		"isolation", opts.Isolation.String(),
		"read_only", opts.ReadOnly,
	)

	return &Tx{
		tx:      tx,
		pool:    p,
		start:   time.Now(),
		options: opts,
	}, nil
}

// Tx 返回底层事务
func (t *Tx) Tx() *sql.Tx {
	return t.tx
}

// Commit 提交事务
func (t *Tx) Commit() error {
	if t.done {
		return fmt.Errorf("transaction already finished")
	}
	t.done = true

	err := t.tx.Commit()
	duration := time.Since(t.start)

	if err != nil {
		logger.Error("failed to commit transaction",
			"duration", duration,
			"error", err,
		)
		return fmt.Errorf("commit transaction: %w", err)
	}

	logger.Debug("transaction committed",
		"duration", duration,
	)

	return nil
}

// Rollback 回滚事务
func (t *Tx) Rollback() error {
	if t.done {
		return nil // 已经完成的事务，忽略回滚
	}
	t.done = true

	err := t.tx.Rollback()
	duration := time.Since(t.start)

	if err != nil {
		logger.Error("failed to rollback transaction",
			"duration", duration,
			"error", err,
		)
		return fmt.Errorf("rollback transaction: %w", err)
	}

	logger.Debug("transaction rolled back",
		"duration", duration,
	)

	return nil
}

// ExecContext 在事务中执行 SQL
func (t *Tx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()

	result, err := t.tx.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	t.pool.logQuery(ctx, "tx_exec", query, duration, err)

	return result, err
}

// QueryContext 在事务中执行查询
func (t *Tx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()

	rows, err := t.tx.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	t.pool.logQuery(ctx, "tx_query", query, duration, err)

	return rows, err
}

// QueryRowContext 在事务中执行单行查询
func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()

	row := t.tx.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)

	t.pool.logQuery(ctx, "tx_query_row", query, duration, nil)

	return row
}

// TxFunc 事务函数类型
type TxFunc func(ctx context.Context, tx *Tx) error

// WithTx 在事务中执行函数，自动提交或回滚
func (p *Pool) WithTx(ctx context.Context, fn TxFunc) error {
	return p.WithTxOptions(ctx, DefaultTxOptions(), fn)
}

// WithTxOptions 在事务中执行函数，支持自定义选项
func (p *Pool) WithTxOptions(ctx context.Context, opts *TxOptions, fn TxFunc) error {
	tx, err := p.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	err = fn(ctx, tx)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			logger.Error("rollback failed after error",
				"error", rbErr,
				"original_error", err,
			)
		}
		return err
	}

	return tx.Commit()
}

// WithReadOnlyTx 在只读事务中执行函数
func (p *Pool) WithReadOnlyTx(ctx context.Context, fn TxFunc) error {
	opts := DefaultTxOptions()
	opts.ReadOnly = true
	return p.WithTxOptions(ctx, opts, fn)
}

// WithSerializableTx 在可序列化事务中执行函数
func (p *Pool) WithSerializableTx(ctx context.Context, fn TxFunc) error {
	opts := DefaultTxOptions()
	opts.Isolation = sql.LevelSerializable
	return p.WithTxOptions(ctx, opts, fn)
}

// PrepareContext 在事务中准备语句
func (t *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.tx.PrepareContext(ctx, query)
}
