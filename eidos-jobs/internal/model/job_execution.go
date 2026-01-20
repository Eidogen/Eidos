package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JobStatus 任务执行状态
type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusSuccess JobStatus = "success"
	JobStatusFailed  JobStatus = "failed"
	JobStatusSkipped JobStatus = "skipped"
)

// JobExecution 任务执行记录
type JobExecution struct {
	ID           int64      `gorm:"column:id;primaryKey;autoIncrement"`
	JobName      string     `gorm:"column:job_name;type:varchar(100);not null"`
	Status       JobStatus  `gorm:"column:status;type:varchar(20);not null"`
	StartedAt    int64      `gorm:"column:started_at;not null"`
	FinishedAt   *int64     `gorm:"column:finished_at"`
	DurationMs   *int       `gorm:"column:duration_ms"`
	ErrorMessage *string    `gorm:"column:error_message;type:text"`
	Result       JSONResult `gorm:"column:result;type:jsonb"`
	CreatedAt    int64      `gorm:"column:created_at;not null"`
}

// TableName 表名
func (JobExecution) TableName() string {
	return "jobs_executions"
}

// JSONResult JSON 结果类型
type JSONResult map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSONResult) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口
func (j *JSONResult) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}
