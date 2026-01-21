package app

import (
	"log/slog"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/migrate"
	"github.com/eidos-exchange/eidos/eidos-market/migrations"
	"gorm.io/gorm"
)

// AutoMigrate 自动执行数据库迁移
func AutoMigrate(db *gorm.DB, logger *slog.Logger) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	migrator := migrate.NewMigrator(sqlDB, "eidos-market", logger)
	if err := migrator.AutoMigrate(migrations.FS, "."); err != nil {
		logger.Error("auto migration failed", "error", err)
		return err
	}

	return nil
}
