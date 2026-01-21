// Package client 归档数据提供者实现
package client

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
)

// ArchiveDataProviderImpl 归档数据提供者实现
// 直接操作数据库进行数据归档
type ArchiveDataProviderImpl struct {
	db            *gorm.DB
	archiveDB     *gorm.DB // 归档数据库 (可选，如果不同于主库)
	archivePolicy map[string]ArchivePolicy
}

// ArchivePolicy 归档策略
type ArchivePolicy struct {
	// TimeColumn 时间列名
	TimeColumn string
	// IDColumn ID 列名
	IDColumn string
	// ArchiveTableName 归档表名 (为空则直接删除)
	ArchiveTableName string
	// Condition 额外的过滤条件
	Condition string
}

// DefaultArchivePolicies 默认归档策略
var DefaultArchivePolicies = map[string]ArchivePolicy{
	"orders": {
		TimeColumn:       "created_at",
		IDColumn:         "id",
		ArchiveTableName: "orders_archive",
		Condition:        "status IN ('FILLED', 'CANCELLED', 'EXPIRED')",
	},
	"trades": {
		TimeColumn:       "created_at",
		IDColumn:         "id",
		ArchiveTableName: "trades_archive",
		Condition:        "",
	},
	"balance_logs": {
		TimeColumn:       "created_at",
		IDColumn:         "id",
		ArchiveTableName: "", // 直接删除，不归档
		Condition:        "",
	},
	"order_events": {
		TimeColumn:       "created_at",
		IDColumn:         "id",
		ArchiveTableName: "", // 直接删除，不归档
		Condition:        "",
	},
}

// NewArchiveDataProvider 创建归档数据提供者
func NewArchiveDataProvider(db *gorm.DB, archiveDB *gorm.DB) *ArchiveDataProviderImpl {
	if archiveDB == nil {
		archiveDB = db
	}
	return &ArchiveDataProviderImpl{
		db:            db,
		archiveDB:     archiveDB,
		archivePolicy: DefaultArchivePolicies,
	}
}

// SetArchivePolicy 设置归档策略
func (p *ArchiveDataProviderImpl) SetArchivePolicy(tableName string, policy ArchivePolicy) {
	p.archivePolicy[tableName] = policy
}

// GetArchivableRecords 获取可归档的记录
func (p *ArchiveDataProviderImpl) GetArchivableRecords(ctx context.Context, tableName string, beforeTime int64, lastID int64, limit int) ([]int64, error) {
	policy, ok := p.archivePolicy[tableName]
	if !ok {
		policy = ArchivePolicy{
			TimeColumn: "created_at",
			IDColumn:   "id",
		}
	}

	// 构建查询
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s < ? AND %s > ?",
		policy.IDColumn, tableName, policy.TimeColumn, policy.IDColumn,
	)

	// 添加额外条件
	if policy.Condition != "" {
		query += " AND " + policy.Condition
	}

	query += fmt.Sprintf(" ORDER BY %s ASC LIMIT ?", policy.IDColumn)

	var ids []int64
	if err := p.db.WithContext(ctx).Raw(query, beforeTime, lastID, limit).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("query archivable records: %w", err)
	}

	return ids, nil
}

// ArchiveRecords 归档记录
func (p *ArchiveDataProviderImpl) ArchiveRecords(ctx context.Context, tableName string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	policy, ok := p.archivePolicy[tableName]
	if !ok {
		policy = ArchivePolicy{
			IDColumn: "id",
		}
	}

	// 开始事务
	tx := p.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// 如果有归档表，先复制数据
	if policy.ArchiveTableName != "" {
		// 使用 INSERT ... SELECT 复制数据到归档表
		insertQuery := fmt.Sprintf(
			"INSERT INTO %s SELECT * FROM %s WHERE %s IN ?",
			policy.ArchiveTableName, tableName, policy.IDColumn,
		)
		if err := tx.Exec(insertQuery, ids).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("copy to archive table: %w", err)
		}

		logger.Debug("copied records to archive table",
			"source_table", tableName,
			"archive_table", policy.ArchiveTableName,
			"count", len(ids))
	}

	// 从源表删除数据
	deleteQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN ?",
		tableName, policy.IDColumn,
	)
	if err := tx.Exec(deleteQuery, ids).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("delete from source table: %w", err)
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	logger.Debug("archived records",
		"table", tableName,
		"count", len(ids))

	return nil
}

// GetMaxID 获取表中最大 ID
func (p *ArchiveDataProviderImpl) GetMaxID(ctx context.Context, tableName string, beforeTime int64) (int64, error) {
	policy, ok := p.archivePolicy[tableName]
	if !ok {
		policy = ArchivePolicy{
			TimeColumn: "created_at",
			IDColumn:   "id",
		}
	}

	query := fmt.Sprintf(
		"SELECT COALESCE(MAX(%s), 0) FROM %s WHERE %s < ?",
		policy.IDColumn, tableName, policy.TimeColumn,
	)

	if policy.Condition != "" {
		query = fmt.Sprintf(
			"SELECT COALESCE(MAX(%s), 0) FROM %s WHERE %s < ? AND %s",
			policy.IDColumn, tableName, policy.TimeColumn, policy.Condition,
		)
	}

	var maxID int64
	if err := p.db.WithContext(ctx).Raw(query, beforeTime).Scan(&maxID).Error; err != nil {
		return 0, fmt.Errorf("query max id: %w", err)
	}

	return maxID, nil
}

// Ensure ArchiveDataProviderImpl implements jobs.ArchiveDataProvider
var _ jobs.ArchiveDataProvider = (*ArchiveDataProviderImpl)(nil)
