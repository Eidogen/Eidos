// Package migrate 提供数据库自动迁移功能 (基于 golang-migrate)
package migrate

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"
)

// Migrator 迁移器
type Migrator struct {
	db          *sql.DB
	logger      *zap.Logger
	serviceName string
}

// NewMigrator 创建迁移器
func NewMigrator(db *sql.DB, serviceName string, logger *zap.Logger) *Migrator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Migrator{
		db:          db,
		logger:      logger,
		serviceName: serviceName,
	}
}

// AutoMigrate 从 embed.FS 自动执行迁移
// migrationsFS 应该是包含迁移文件的 embed.FS
// migrationsPath 是 embed.FS 中迁移文件的路径，如 "migrations"
func (m *Migrator) AutoMigrate(migrationsFS embed.FS, migrationsPath string) error {
	m.logger.Info("starting auto migration",
		zap.String("service", m.serviceName),
		zap.String("path", migrationsPath))

	// 创建 iofs 源
	source, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		return fmt.Errorf("create migration source failed: %w", err)
	}

	// 创建 postgres 驱动
	driver, err := postgres.WithInstance(m.db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create postgres driver failed: %w", err)
	}

	// 创建迁移实例
	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator failed: %w", err)
	}
	defer migrator.Close()

	// 执行迁移
	if err := migrator.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info("no new migrations to apply", zap.String("service", m.serviceName))
			return nil
		}
		return fmt.Errorf("migration failed: %w", err)
	}

	// 获取当前版本
	version, dirty, err := migrator.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("get migration version failed: %w", err)
	}

	m.logger.Info("auto migration completed",
		zap.String("service", m.serviceName),
		zap.Uint("version", version),
		zap.Bool("dirty", dirty))

	return nil
}

// Rollback 回滚一个版本
func (m *Migrator) Rollback(migrationsFS embed.FS, migrationsPath string) error {
	m.logger.Info("starting rollback",
		zap.String("service", m.serviceName),
		zap.String("path", migrationsPath))

	// 创建 iofs 源
	source, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		return fmt.Errorf("create migration source failed: %w", err)
	}

	// 创建 postgres 驱动
	driver, err := postgres.WithInstance(m.db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create postgres driver failed: %w", err)
	}

	// 创建迁移实例
	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator failed: %w", err)
	}
	defer migrator.Close()

	// 回滚一步
	if err := migrator.Steps(-1); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info("no migrations to rollback", zap.String("service", m.serviceName))
			return nil
		}
		return fmt.Errorf("rollback failed: %w", err)
	}

	// 获取当前版本
	version, dirty, err := migrator.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("get migration version failed: %w", err)
	}

	m.logger.Info("rollback completed",
		zap.String("service", m.serviceName),
		zap.Uint("version", version),
		zap.Bool("dirty", dirty))

	return nil
}

// MigrateToVersion 迁移到指定版本
func (m *Migrator) MigrateToVersion(migrationsFS embed.FS, migrationsPath string, version uint) error {
	m.logger.Info("migrating to version",
		zap.String("service", m.serviceName),
		zap.Uint("target_version", version))

	// 创建 iofs 源
	source, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		return fmt.Errorf("create migration source failed: %w", err)
	}

	// 创建 postgres 驱动
	driver, err := postgres.WithInstance(m.db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create postgres driver failed: %w", err)
	}

	// 创建迁移实例
	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator failed: %w", err)
	}
	defer migrator.Close()

	// 迁移到指定版本
	if err := migrator.Migrate(version); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info("already at target version", zap.String("service", m.serviceName))
			return nil
		}
		return fmt.Errorf("migration to version %d failed: %w", version, err)
	}

	m.logger.Info("migration to version completed",
		zap.String("service", m.serviceName),
		zap.Uint("version", version))

	return nil
}

// GetVersion 获取当前迁移版本
func (m *Migrator) GetVersion(migrationsFS embed.FS, migrationsPath string) (uint, bool, error) {
	// 创建 iofs 源
	source, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		return 0, false, fmt.Errorf("create migration source failed: %w", err)
	}

	// 创建 postgres 驱动
	driver, err := postgres.WithInstance(m.db, &postgres.Config{})
	if err != nil {
		return 0, false, fmt.Errorf("create postgres driver failed: %w", err)
	}

	// 创建迁移实例
	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return 0, false, fmt.Errorf("create migrator failed: %w", err)
	}
	defer migrator.Close()

	version, dirty, err := migrator.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, err
	}

	return version, dirty, nil
}
