// Package main 提供数据库迁移命令行工具
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// MigrationRecord 迁移记录表
type MigrationRecord struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Version   string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Name      string `gorm:"type:varchar(255);not null"`
	AppliedAt int64  `gorm:"type:bigint;not null"`
}

// TableName 返回表名
func (MigrationRecord) TableName() string {
	return "schema_migrations"
}

// Migration 单个迁移
type Migration struct {
	Version string
	Name    string
	UpSQL   string
	DownSQL string
}

func main() {
	// 解析命令行参数
	var (
		command    string
		migrateDir string
		configPath string
		dsn        string
	)

	flag.StringVar(&command, "cmd", "up", "Command: up, down, status, create")
	flag.StringVar(&migrateDir, "dir", "migrations", "Migrations directory")
	flag.StringVar(&configPath, "config", "", "Config file path (optional)")
	flag.StringVar(&dsn, "dsn", "", "Database DSN (overrides config)")
	flag.Parse()

	// 初始化日志
	if err := logger.Init(&logger.Config{
		Level:       "info",
		Format:      "console",
		ServiceName: "migrate",
	}); err != nil {
		fmt.Printf("init logger failed: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 获取数据库连接字符串
	if dsn == "" {
		cfg, err := config.Load()
		if err != nil {
			logger.Fatal("load config failed", "error", err)
		}
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			cfg.Database.Host,
			cfg.Database.Port,
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Database,
		)
	}

	// 连接数据库
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		logger.Fatal("connect database failed", "error", err)
	}

	// 确保迁移记录表存在
	if err := db.AutoMigrate(&MigrationRecord{}); err != nil {
		logger.Fatal("create migration table failed", "error", err)
	}

	ctx := context.Background()

	switch command {
	case "up":
		if err := runUp(ctx, db, migrateDir); err != nil {
			logger.Fatal("migration up failed", "error", err)
		}
		logger.Info("all migrations applied successfully")

	case "down":
		if err := runDown(ctx, db, migrateDir); err != nil {
			logger.Fatal("migration down failed", "error", err)
		}
		logger.Info("migration rolled back successfully")

	case "status":
		if err := runStatus(ctx, db, migrateDir); err != nil {
			logger.Fatal("get migration status failed", "error", err)
		}

	case "create":
		name := flag.Arg(0)
		if name == "" {
			logger.Fatal("migration name required, usage: -cmd create <name>")
		}
		if err := createMigration(migrateDir, name); err != nil {
			logger.Fatal("create migration failed", "error", err)
		}

	default:
		logger.Fatal("unknown command", "command", command)
	}
}

// loadMigrations 从目录加载迁移文件
func loadMigrations(dir string) ([]Migration, error) {
	files := make(map[string]*Migration)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir failed: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(filename, ".sql") {
			continue
		}

		// 解析文件名: {version}_{name}.{up|down}.sql
		parts := strings.SplitN(filename, "_", 2)
		if len(parts) < 2 {
			continue
		}

		version := parts[0]
		rest := parts[1]

		var direction string
		var name string
		if strings.HasSuffix(rest, ".up.sql") {
			direction = "up"
			name = strings.TrimSuffix(rest, ".up.sql")
		} else if strings.HasSuffix(rest, ".down.sql") {
			direction = "down"
			name = strings.TrimSuffix(rest, ".down.sql")
		} else {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			return nil, fmt.Errorf("read file %s failed: %w", filename, err)
		}

		key := version + "_" + name
		if _, exists := files[key]; !exists {
			files[key] = &Migration{
				Version: version,
				Name:    name,
			}
		}

		if direction == "up" {
			files[key].UpSQL = string(content)
		} else {
			files[key].DownSQL = string(content)
		}
	}

	// 转换为切片并排序
	var migrations []Migration
	for _, m := range files {
		if m.UpSQL != "" {
			migrations = append(migrations, *m)
		}
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// getAppliedMigrations 获取已应用的迁移
func getAppliedMigrations(ctx context.Context, db *gorm.DB) (map[string]*MigrationRecord, error) {
	var records []MigrationRecord
	if err := db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query migrations failed: %w", err)
	}

	result := make(map[string]*MigrationRecord)
	for i := range records {
		result[records[i].Version+"_"+records[i].Name] = &records[i]
	}

	return result, nil
}

// runUp 执行所有未应用的迁移
func runUp(ctx context.Context, db *gorm.DB, dir string) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return err
	}

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		key := migration.Version + "_" + migration.Name
		if _, ok := applied[key]; ok {
			logger.Info("skip applied migration", "version", migration.Version, "name", migration.Name)
			continue
		}

		logger.Info("applying migration", "version", migration.Version, "name", migration.Name)

		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 执行 SQL
			if err := tx.Exec(migration.UpSQL).Error; err != nil {
				return fmt.Errorf("exec up sql failed: %w", err)
			}

			// 记录迁移
			record := &MigrationRecord{
				Version:   migration.Version,
				Name:      migration.Name,
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

		logger.Info("migration applied", "version", migration.Version, "name", migration.Name)
	}

	return nil
}

// runDown 回滚最后一个迁移
func runDown(ctx context.Context, db *gorm.DB, dir string) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return err
	}

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	// 找到最后应用的迁移
	var lastMigration *Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		key := migrations[i].Version + "_" + migrations[i].Name
		if _, ok := applied[key]; ok {
			lastMigration = &migrations[i]
			break
		}
	}

	if lastMigration == nil {
		return fmt.Errorf("no matching migration found")
	}

	key := lastMigration.Version + "_" + lastMigration.Name
	if lastMigration.DownSQL == "" {
		return fmt.Errorf("migration %s has no down script", key)
	}

	logger.Info("rolling back migration", "version", lastMigration.Version, "name", lastMigration.Name)

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 执行回滚 SQL
		if err := tx.Exec(lastMigration.DownSQL).Error; err != nil {
			return fmt.Errorf("exec down sql failed: %w", err)
		}

		// 删除迁移记录
		if err := tx.Where("version = ? AND name = ?", lastMigration.Version, lastMigration.Name).
			Delete(&MigrationRecord{}).Error; err != nil {
			return fmt.Errorf("delete migration record failed: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("rollback migration %s failed: %w", key, err)
	}

	return nil
}

// runStatus 显示迁移状态
func runStatus(ctx context.Context, db *gorm.DB, dir string) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return err
	}

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Migration Status ===")
	fmt.Printf("%-10s %-30s %-10s %-25s\n", "VERSION", "NAME", "STATUS", "APPLIED AT")
	fmt.Println(strings.Repeat("-", 80))

	for _, migration := range migrations {
		key := migration.Version + "_" + migration.Name
		status := "pending"
		appliedAt := ""

		if record, ok := applied[key]; ok {
			status = "applied"
			appliedAt = time.UnixMilli(record.AppliedAt).Format("2006-01-02 15:04:05")
		}

		fmt.Printf("%-10s %-30s %-10s %-25s\n", migration.Version, migration.Name, status, appliedAt)
	}

	fmt.Println()
	return nil
}

// createMigration 创建新的迁移文件
func createMigration(dir string, name string) error {
	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir failed: %w", err)
	}

	// 生成版本号 (时间戳)
	version := time.Now().Format("20060102150405")

	// 创建文件
	upFile := filepath.Join(dir, fmt.Sprintf("%s_%s.up.sql", version, name))
	downFile := filepath.Join(dir, fmt.Sprintf("%s_%s.down.sql", version, name))

	upContent := fmt.Sprintf("-- Migration: %s\n-- Description: %s\n-- Created: %s\n\n-- Add your SQL here\n",
		version, name, time.Now().Format("2006-01-02 15:04:05"))
	downContent := fmt.Sprintf("-- Rollback: %s\n-- Description: %s\n-- Created: %s\n\n-- Add your rollback SQL here\n",
		version, name, time.Now().Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(upFile, []byte(upContent), 0644); err != nil {
		return fmt.Errorf("create up file failed: %w", err)
	}

	if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
		return fmt.Errorf("create down file failed: %w", err)
	}

	logger.Info("created migration files",
		"up", upFile,
		"down", downFile)

	return nil
}

// Unused variable to satisfy import
var _ fs.FS
