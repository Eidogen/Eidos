package app

import (
	"github.com/eidos-exchange/eidos/eidos-admin/migrations"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/migrate"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AutoMigrate 自动执行数据库迁移
func AutoMigrate(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	migrator := migrate.NewMigrator(sqlDB, "eidos-admin", logger.L())
	if err := migrator.AutoMigrate(migrations.FS, "."); err != nil {
		logger.Error("auto migration failed", zap.Error(err))
		return err
	}

	return nil
}
