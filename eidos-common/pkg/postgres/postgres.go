// Package postgres 提供 PostgreSQL 数据库连接池、事务管理和查询构建功能
//
// 基本使用:
//
//	cfg := &postgres.Config{
//		Host:     "localhost",
//		Port:     5432,
//		Database: "mydb",
//		User:     "user",
//		Password: "pass",
//	}
//
//	pool, err := postgres.NewPool(cfg)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer pool.Close()
//
//	// 查询
//	rows, err := pool.QueryContext(ctx, "SELECT * FROM users WHERE id = $1", 1)
//
// 事务使用:
//
//	err := pool.WithTx(ctx, func(ctx context.Context, tx *postgres.Tx) error {
//		_, err := tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", 100, fromID)
//		if err != nil {
//			return err
//		}
//		_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", 100, toID)
//		return err
//	})
//
// 查询构建器:
//
//	query, args := postgres.NewBuilder("users").
//		Select("id", "name", "email").
//		Where("status = ?", "active").
//		Where("created_at > ?", time.Now().AddDate(0, -1, 0)).
//		OrderByDesc("created_at").
//		Limit(10).
//		BuildSelect()
package postgres

import (
	"database/sql"

	_ "github.com/lib/pq"
)

// Version 包版本
const Version = "1.0.0"

// Executor 查询执行器接口，Pool 和 Tx 都实现此接口
type Executor interface {
	QueryExecutor
}

// 确保 Pool 和 Tx 实现 Executor 接口
var (
	_ Executor = (*Pool)(nil)
	_ Executor = (*Tx)(nil)
)

// Scanner 行扫描器接口
type Scanner interface {
	Scan(dest ...interface{}) error
}

// Scannable 可扫描接口
type Scannable interface {
	ScanRow(row Scanner) error
}

// NullString 可空字符串，sql.NullString 的别名
type NullString = sql.NullString

// NullInt64 可空整数，sql.NullInt64 的别名
type NullInt64 = sql.NullInt64

// NullFloat64 可空浮点数，sql.NullFloat64 的别名
type NullFloat64 = sql.NullFloat64

// NullBool 可空布尔，sql.NullBool 的别名
type NullBool = sql.NullBool

// NullTime 可空时间，sql.NullTime 的别名
type NullTime = sql.NullTime

// IsolationLevel 事务隔离级别
type IsolationLevel = sql.IsolationLevel

// 事务隔离级别常量
const (
	LevelDefault         = sql.LevelDefault
	LevelReadUncommitted = sql.LevelReadUncommitted
	LevelReadCommitted   = sql.LevelReadCommitted
	LevelRepeatableRead  = sql.LevelRepeatableRead
	LevelSerializable    = sql.LevelSerializable
)
