package app

import (
	"github.com/eidos-exchange/eidos/eidos-admin/migrations"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/migrate"
	"gorm.io/gorm"
)

// EnsureDatabase 确保数据库存在
func EnsureDatabase(host string, port int, user, password, dbname string) error {
	return migrate.EnsureDatabase(host, port, user, password, dbname)
}

// AutoMigrate 自动执行数据库迁移
func AutoMigrate(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	migrator := migrate.NewMigrator(sqlDB, "eidos-admin", logger.L())
	if err := migrator.AutoMigrate(migrations.FS, "."); err != nil {
		logger.Error("auto migration failed", "error", err)
		return err
	}

	return nil
}
