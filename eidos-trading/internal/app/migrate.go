package app

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SchemaMigration 迁移记录
type SchemaMigration struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Version   string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Name      string `gorm:"type:varchar(255);not null"`
	AppliedAt int64  `gorm:"type:bigint;not null"`
}

// TableName 返回表名
func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

// AutoMigrate 自动执行 SQL 迁移
func AutoMigrate(db *gorm.DB) error {
	// 确保迁移记录表存在
	if err := db.AutoMigrate(&SchemaMigration{}); err != nil {
		return fmt.Errorf("create schema_migrations table failed: %w", err)
	}

	// 读取迁移目录
	migrateDir := "migrations"
	entries, err := os.ReadDir(migrateDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("migrations directory not found, skipping migrations")
			return nil
		}
		return fmt.Errorf("read migrations dir failed: %w", err)
	}

	// 收集迁移文件
	type migration struct {
		version string
		name    string
		sql     string
	}
	migrations := make(map[string]*migration)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if !strings.HasSuffix(filename, ".up.sql") {
			continue
		}

		parts := strings.SplitN(filename, "_", 2)
		if len(parts) < 2 {
			continue
		}
		version := parts[0]
		name := strings.TrimSuffix(parts[1], ".up.sql")

		content, err := os.ReadFile(fmt.Sprintf("%s/%s", migrateDir, filename))
		if err != nil {
			return fmt.Errorf("read migration file %s failed: %w", filename, err)
		}

		key := version + "_" + name
		migrations[key] = &migration{
			version: version,
			name:    name,
			sql:     string(content),
		}
	}

	// 获取已应用的迁移
	var applied []SchemaMigration
	if err := db.Find(&applied).Error; err != nil {
		return fmt.Errorf("query applied migrations failed: %w", err)
	}
	appliedMap := make(map[string]bool)
	for _, m := range applied {
		appliedMap[m.Version+"_"+m.Name] = true
	}

	// 按版本排序执行
	var keys []string
	for k := range migrations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if appliedMap[key] {
			continue
		}

		m := migrations[key]
		logger.Info("applying migration", zap.String("version", m.version), zap.String("name", m.name))

		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(m.sql).Error; err != nil {
				return fmt.Errorf("exec migration sql failed: %w", err)
			}

			record := &SchemaMigration{
				Version:   m.version,
				Name:      m.name,
				AppliedAt: time.Now().UnixMilli(),
			}
			if err := tx.Create(record).Error; err != nil {
				return fmt.Errorf("create migration record failed: %w", err)
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("apply migration %s failed: %w", key, err)
		}

		logger.Info("migration applied", zap.String("version", m.version), zap.String("name", m.name))
	}

	return nil
}
