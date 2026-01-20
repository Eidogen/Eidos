// Package migrations 嵌入数据库迁移文件
package migrations

import "embed"

// FS 嵌入的迁移文件系统
//
//go:embed *.sql
var FS embed.FS
