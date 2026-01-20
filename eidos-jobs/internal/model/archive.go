package model

// ArchiveProgress 归档进度
type ArchiveProgress struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SourceTable  string `gorm:"column:table_name;type:varchar(50);not null;uniqueIndex"`
	LastID       int64  `gorm:"column:last_id;not null;default:0"`
	StartedAt    *int64 `gorm:"column:started_at"`
	UpdatedAt    int64  `gorm:"column:updated_at;not null"`
}

// TableName 表名
func (ArchiveProgress) TableName() string {
	return "jobs_archive_progress"
}

// ArchiveConfig 归档配置
type ArchiveConfig struct {
	// RetentionDays 保留天数
	RetentionDays int
	// BatchSize 批量处理大小
	BatchSize int
	// SleepMs 批次间休眠毫秒数
	SleepMs int
	// ArchiveBeforeDrop 删除前是否归档
	ArchiveBeforeDrop bool
}

// DefaultArchiveConfig 默认归档配置
var DefaultArchiveConfig = ArchiveConfig{
	RetentionDays:     30,
	BatchSize:         1000,
	SleepMs:           100,
	ArchiveBeforeDrop: false,
}

// ArchivableTables 可归档的表配置
var ArchivableTables = map[string]ArchiveConfig{
	"orders": {
		RetentionDays:     30,
		BatchSize:         1000,
		SleepMs:           100,
		ArchiveBeforeDrop: true,
	},
	"trades": {
		RetentionDays:     30,
		BatchSize:         1000,
		SleepMs:           100,
		ArchiveBeforeDrop: true,
	},
}
