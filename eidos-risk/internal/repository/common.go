package repository

import (
	"strings"
)

// Pagination 分页参数
type Pagination struct {
	Page     int
	PageSize int
}

// Offset 计算偏移量
func (p *Pagination) Offset() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	return (p.Page - 1) * p.Limit()
}

// Limit 获取每页大小
func (p *Pagination) Limit() int {
	if p.PageSize <= 0 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p.PageSize
}

// NewPagination 创建分页参数
func NewPagination(page, pageSize int) *Pagination {
	return &Pagination{
		Page:     page,
		PageSize: pageSize,
	}
}

// isDuplicateKeyError 检查是否是唯一约束冲突错误
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "unique_violation") ||
		strings.Contains(errStr, "23505")
}
