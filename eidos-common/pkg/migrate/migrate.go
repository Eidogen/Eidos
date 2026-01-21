// Package migrate 提供数据库自动迁移功能 (基于 golang-migrate)
//
// 使用方式:
//
//  1. 基于 Migrator 的迁移（推荐，微服务架构）:
//     migrator := migrate.NewMigrator(db, "service-name", logger)
//     err := migrator.AutoMigrate(migrations.FS, ".")
//
//  2. 基于 DSN 的简单迁移（单体应用）:
//     err := migrate.Run(dsn, migrationsFS, "migrations")
//
//  3. 确保数据库存在:
//     err := migrate.EnsureDatabase(host, port, user, password, dbname)
package migrate

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

// EnsureDatabase 确保目标数据库存在，如果不存在则创建
func EnsureDatabase(host string, port int, user, password, dbname string) error {
	// 1. 先连接到 postgres 默认库
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable", host, port, user, password)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("connect to postgres failed: %w", err)
	}
	defer db.Close()

	// 2. 检查数据库是否存在
	var exists bool
	query := fmt.Sprintf("SELECT EXISTS (SELECT FROM pg_database WHERE datname = '%s')", dbname)
	err = db.QueryRow(query).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check database existence failed: %w", err)
	}

	if exists {
		return nil
	}

	// 3. 创建数据库
	// 注意: CREATE DATABASE 不能在事务中执行，且不能使用参数化查询（针对库名）
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbname))
	if err != nil {
		return fmt.Errorf("create database %s failed: %w", dbname, err)
	}

	return nil
}

// Migrator 迁移器
type Migrator struct {
	db          *sql.DB
	logger      *slog.Logger
	serviceName string
}

// NewMigrator 创建迁移器
func NewMigrator(db *sql.DB, serviceName string, logger *slog.Logger) *Migrator {
	if logger == nil {
		logger = slog.Default()
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
		"service", m.serviceName,
		"path", migrationsPath)

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
			m.logger.Info("no new migrations to apply", "service", m.serviceName)
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
		"service", m.serviceName,
		"version", version,
		"dirty", dirty)

	return nil
}

// Rollback 回滚一个版本
func (m *Migrator) Rollback(migrationsFS embed.FS, migrationsPath string) error {
	m.logger.Info("starting rollback",
		"service", m.serviceName,
		"path", migrationsPath)

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
			m.logger.Info("no migrations to rollback", "service", m.serviceName)
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
		"service", m.serviceName,
		"version", version,
		"dirty", dirty)

	return nil
}

// MigrateToVersion 迁移到指定版本
func (m *Migrator) MigrateToVersion(migrationsFS embed.FS, migrationsPath string, version uint) error {
	m.logger.Info("migrating to version",
		"service", m.serviceName,
		"target_version", version)

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
			m.logger.Info("already at target version", "service", m.serviceName)
			return nil
		}
		return fmt.Errorf("migration to version %d failed: %w", version, err)
	}

	m.logger.Info("migration to version completed",
		"service", m.serviceName,
		"version", version)

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

// Run 执行数据库迁移（简洁 API，适用于单体应用）
// dsn: PostgreSQL 连接字符串，如 postgres://user:pass@host:5432/dbname?sslmode=disable
// migrationsFS: 包含迁移文件的 embed.FS
// migrationsPath: embed.FS 中迁移文件的路径
func Run(dsn string, migrationsFS embed.FS, migrationsPath string) error {
	source, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		return fmt.Errorf("create migration source failed: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return fmt.Errorf("create migrator failed: %w", err)
	}
	defer m.Close()

	// 获取当前版本
	version, dirty, _ := m.Version()
	slog.Info("migration status", "current_version", version, "dirty", dirty)

	// 执行迁移
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("no new migrations to apply")
			return nil
		}
		return fmt.Errorf("migration failed: %w", err)
	}

	// 获取新版本
	newVersion, _, _ := m.Version()
	slog.Info("migration completed", "new_version", newVersion)

	return nil
}

// RollbackDSN 回滚最后一次迁移（简洁 API）
func RollbackDSN(dsn string, migrationsFS embed.FS, migrationsPath string) error {
	source, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		return fmt.Errorf("create migration source failed: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return fmt.Errorf("create migrator failed: %w", err)
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	version, _, _ := m.Version()
	slog.Info("rollback completed", "version", version)

	return nil
}
