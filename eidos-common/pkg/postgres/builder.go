package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Builder SQL 查询构建器
type Builder struct {
	table      string
	columns    []string
	conditions []condition
	orderBy    []string
	groupBy    []string
	having     []condition
	limit      int
	offset     int
	args       []interface{}
	argIndex   int
}

type condition struct {
	clause string
	args   []interface{}
}

// NewBuilder 创建查询构建器
func NewBuilder(table string) *Builder {
	return &Builder{
		table:    table,
		argIndex: 1,
	}
}

// Select 设置查询列
func (b *Builder) Select(columns ...string) *Builder {
	b.columns = columns
	return b
}

// Where 添加 WHERE 条件
func (b *Builder) Where(clause string, args ...interface{}) *Builder {
	b.conditions = append(b.conditions, condition{
		clause: b.rewritePlaceholders(clause),
		args:   args,
	})
	return b
}

// WhereIn 添加 IN 条件
func (b *Builder) WhereIn(column string, values []interface{}) *Builder {
	if len(values) == 0 {
		b.conditions = append(b.conditions, condition{
			clause: "1 = 0", // 总是 false
			args:   nil,
		})
		return b
	}

	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = "$" + strconv.Itoa(b.argIndex)
		b.argIndex++
	}

	clause := fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", "))
	b.conditions = append(b.conditions, condition{
		clause: clause,
		args:   values,
	})
	return b
}

// WhereNotIn 添加 NOT IN 条件
func (b *Builder) WhereNotIn(column string, values []interface{}) *Builder {
	if len(values) == 0 {
		return b // 如果值为空，不添加条件
	}

	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = "$" + strconv.Itoa(b.argIndex)
		b.argIndex++
	}

	clause := fmt.Sprintf("%s NOT IN (%s)", column, strings.Join(placeholders, ", "))
	b.conditions = append(b.conditions, condition{
		clause: clause,
		args:   values,
	})
	return b
}

// WhereBetween 添加 BETWEEN 条件
func (b *Builder) WhereBetween(column string, start, end interface{}) *Builder {
	clause := fmt.Sprintf("%s BETWEEN $%d AND $%d", column, b.argIndex, b.argIndex+1)
	b.argIndex += 2
	b.conditions = append(b.conditions, condition{
		clause: clause,
		args:   []interface{}{start, end},
	})
	return b
}

// WhereNull 添加 IS NULL 条件
func (b *Builder) WhereNull(column string) *Builder {
	b.conditions = append(b.conditions, condition{
		clause: fmt.Sprintf("%s IS NULL", column),
		args:   nil,
	})
	return b
}

// WhereNotNull 添加 IS NOT NULL 条件
func (b *Builder) WhereNotNull(column string) *Builder {
	b.conditions = append(b.conditions, condition{
		clause: fmt.Sprintf("%s IS NOT NULL", column),
		args:   nil,
	})
	return b
}

// WhereLike 添加 LIKE 条件
func (b *Builder) WhereLike(column string, pattern string) *Builder {
	clause := fmt.Sprintf("%s LIKE $%d", column, b.argIndex)
	b.argIndex++
	b.conditions = append(b.conditions, condition{
		clause: clause,
		args:   []interface{}{pattern},
	})
	return b
}

// WhereILike 添加 ILIKE 条件 (大小写不敏感)
func (b *Builder) WhereILike(column string, pattern string) *Builder {
	clause := fmt.Sprintf("%s ILIKE $%d", column, b.argIndex)
	b.argIndex++
	b.conditions = append(b.conditions, condition{
		clause: clause,
		args:   []interface{}{pattern},
	})
	return b
}

// OrWhere 添加 OR WHERE 条件
func (b *Builder) OrWhere(clause string, args ...interface{}) *Builder {
	if len(b.conditions) == 0 {
		return b.Where(clause, args...)
	}

	// 将最后一个条件与新条件用 OR 连接
	last := b.conditions[len(b.conditions)-1]
	newClause := fmt.Sprintf("(%s OR %s)", last.clause, b.rewritePlaceholders(clause))
	b.conditions[len(b.conditions)-1] = condition{
		clause: newClause,
		args:   append(last.args, args...),
	}
	return b
}

// OrderBy 添加排序
func (b *Builder) OrderBy(column string, direction string) *Builder {
	dir := strings.ToUpper(direction)
	if dir != "ASC" && dir != "DESC" {
		dir = "ASC"
	}
	b.orderBy = append(b.orderBy, fmt.Sprintf("%s %s", column, dir))
	return b
}

// OrderByDesc 添加降序排序
func (b *Builder) OrderByDesc(column string) *Builder {
	return b.OrderBy(column, "DESC")
}

// GroupBy 添加分组
func (b *Builder) GroupBy(columns ...string) *Builder {
	b.groupBy = append(b.groupBy, columns...)
	return b
}

// Having 添加 HAVING 条件
func (b *Builder) Having(clause string, args ...interface{}) *Builder {
	b.having = append(b.having, condition{
		clause: b.rewritePlaceholders(clause),
		args:   args,
	})
	return b
}

// Limit 设置限制
func (b *Builder) Limit(limit int) *Builder {
	b.limit = limit
	return b
}

// Offset 设置偏移
func (b *Builder) Offset(offset int) *Builder {
	b.offset = offset
	return b
}

// Page 设置分页 (page 从 1 开始)
func (b *Builder) Page(page, pageSize int) *Builder {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	b.limit = pageSize
	b.offset = (page - 1) * pageSize
	return b
}

// rewritePlaceholders 将 ? 占位符转换为 $1, $2, ...
func (b *Builder) rewritePlaceholders(clause string) string {
	var result strings.Builder
	for i := 0; i < len(clause); i++ {
		if clause[i] == '?' {
			result.WriteString("$")
			result.WriteString(strconv.Itoa(b.argIndex))
			b.argIndex++
		} else {
			result.WriteByte(clause[i])
		}
	}
	return result.String()
}

// BuildSelect 构建 SELECT 语句
func (b *Builder) BuildSelect() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	// SELECT
	sql.WriteString("SELECT ")
	if len(b.columns) == 0 {
		sql.WriteString("*")
	} else {
		sql.WriteString(strings.Join(b.columns, ", "))
	}

	// FROM
	sql.WriteString(" FROM ")
	sql.WriteString(b.table)

	// WHERE
	if len(b.conditions) > 0 {
		sql.WriteString(" WHERE ")
		clauses := make([]string, len(b.conditions))
		for i, cond := range b.conditions {
			clauses[i] = cond.clause
			args = append(args, cond.args...)
		}
		sql.WriteString(strings.Join(clauses, " AND "))
	}

	// GROUP BY
	if len(b.groupBy) > 0 {
		sql.WriteString(" GROUP BY ")
		sql.WriteString(strings.Join(b.groupBy, ", "))
	}

	// HAVING
	if len(b.having) > 0 {
		sql.WriteString(" HAVING ")
		clauses := make([]string, len(b.having))
		for i, cond := range b.having {
			clauses[i] = cond.clause
			args = append(args, cond.args...)
		}
		sql.WriteString(strings.Join(clauses, " AND "))
	}

	// ORDER BY
	if len(b.orderBy) > 0 {
		sql.WriteString(" ORDER BY ")
		sql.WriteString(strings.Join(b.orderBy, ", "))
	}

	// LIMIT
	if b.limit > 0 {
		sql.WriteString(fmt.Sprintf(" LIMIT %d", b.limit))
	}

	// OFFSET
	if b.offset > 0 {
		sql.WriteString(fmt.Sprintf(" OFFSET %d", b.offset))
	}

	return sql.String(), args
}

// BuildCount 构建 COUNT 语句
func (b *Builder) BuildCount() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	sql.WriteString("SELECT COUNT(*) FROM ")
	sql.WriteString(b.table)

	// WHERE
	if len(b.conditions) > 0 {
		sql.WriteString(" WHERE ")
		clauses := make([]string, len(b.conditions))
		for i, cond := range b.conditions {
			clauses[i] = cond.clause
			args = append(args, cond.args...)
		}
		sql.WriteString(strings.Join(clauses, " AND "))
	}

	return sql.String(), args
}

// InsertBuilder INSERT 语句构建器
type InsertBuilder struct {
	table   string
	columns []string
	values  [][]interface{}
	returning []string
	onConflict string
}

// NewInsertBuilder 创建 INSERT 构建器
func NewInsertBuilder(table string) *InsertBuilder {
	return &InsertBuilder{
		table: table,
	}
}

// Columns 设置列名
func (b *InsertBuilder) Columns(columns ...string) *InsertBuilder {
	b.columns = columns
	return b
}

// Values 添加值
func (b *InsertBuilder) Values(values ...interface{}) *InsertBuilder {
	b.values = append(b.values, values)
	return b
}

// Returning 设置返回列
func (b *InsertBuilder) Returning(columns ...string) *InsertBuilder {
	b.returning = columns
	return b
}

// OnConflictDoNothing 设置冲突时不做任何操作
func (b *InsertBuilder) OnConflictDoNothing(columns ...string) *InsertBuilder {
	if len(columns) > 0 {
		b.onConflict = fmt.Sprintf("ON CONFLICT (%s) DO NOTHING", strings.Join(columns, ", "))
	} else {
		b.onConflict = "ON CONFLICT DO NOTHING"
	}
	return b
}

// OnConflictDoUpdate 设置冲突时更新
func (b *InsertBuilder) OnConflictDoUpdate(conflictColumns []string, updateColumns []string) *InsertBuilder {
	updates := make([]string, len(updateColumns))
	for i, col := range updateColumns {
		updates[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
	}
	b.onConflict = fmt.Sprintf("ON CONFLICT (%s) DO UPDATE SET %s",
		strings.Join(conflictColumns, ", "),
		strings.Join(updates, ", "))
	return b
}

// Build 构建 INSERT 语句
func (b *InsertBuilder) Build() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	sql.WriteString("INSERT INTO ")
	sql.WriteString(b.table)
	sql.WriteString(" (")
	sql.WriteString(strings.Join(b.columns, ", "))
	sql.WriteString(") VALUES ")

	argIndex := 1
	valueClauses := make([]string, len(b.values))
	for i, row := range b.values {
		placeholders := make([]string, len(row))
		for j := range row {
			placeholders[j] = "$" + strconv.Itoa(argIndex)
			argIndex++
		}
		valueClauses[i] = "(" + strings.Join(placeholders, ", ") + ")"
		args = append(args, row...)
	}
	sql.WriteString(strings.Join(valueClauses, ", "))

	if b.onConflict != "" {
		sql.WriteString(" ")
		sql.WriteString(b.onConflict)
	}

	if len(b.returning) > 0 {
		sql.WriteString(" RETURNING ")
		sql.WriteString(strings.Join(b.returning, ", "))
	}

	return sql.String(), args
}

// UpdateBuilder UPDATE 语句构建器
type UpdateBuilder struct {
	table      string
	sets       []string
	conditions []condition
	args       []interface{}
	argIndex   int
	returning  []string
}

// NewUpdateBuilder 创建 UPDATE 构建器
func NewUpdateBuilder(table string) *UpdateBuilder {
	return &UpdateBuilder{
		table:    table,
		argIndex: 1,
	}
}

// Set 设置更新值
func (b *UpdateBuilder) Set(column string, value interface{}) *UpdateBuilder {
	b.sets = append(b.sets, fmt.Sprintf("%s = $%d", column, b.argIndex))
	b.args = append(b.args, value)
	b.argIndex++
	return b
}

// SetRaw 设置原始表达式
func (b *UpdateBuilder) SetRaw(column string, expr string) *UpdateBuilder {
	b.sets = append(b.sets, fmt.Sprintf("%s = %s", column, expr))
	return b
}

// Where 添加条件
func (b *UpdateBuilder) Where(clause string, args ...interface{}) *UpdateBuilder {
	// 重写占位符
	var newClause strings.Builder
	for i := 0; i < len(clause); i++ {
		if clause[i] == '?' {
			newClause.WriteString("$")
			newClause.WriteString(strconv.Itoa(b.argIndex))
			b.argIndex++
		} else {
			newClause.WriteByte(clause[i])
		}
	}
	b.conditions = append(b.conditions, condition{
		clause: newClause.String(),
		args:   args,
	})
	return b
}

// Returning 设置返回列
func (b *UpdateBuilder) Returning(columns ...string) *UpdateBuilder {
	b.returning = columns
	return b
}

// Build 构建 UPDATE 语句
func (b *UpdateBuilder) Build() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	sql.WriteString("UPDATE ")
	sql.WriteString(b.table)
	sql.WriteString(" SET ")
	sql.WriteString(strings.Join(b.sets, ", "))

	args = append(args, b.args...)

	if len(b.conditions) > 0 {
		sql.WriteString(" WHERE ")
		clauses := make([]string, len(b.conditions))
		for i, cond := range b.conditions {
			clauses[i] = cond.clause
			args = append(args, cond.args...)
		}
		sql.WriteString(strings.Join(clauses, " AND "))
	}

	if len(b.returning) > 0 {
		sql.WriteString(" RETURNING ")
		sql.WriteString(strings.Join(b.returning, ", "))
	}

	return sql.String(), args
}

// DeleteBuilder DELETE 语句构建器
type DeleteBuilder struct {
	table      string
	conditions []condition
	argIndex   int
	returning  []string
}

// NewDeleteBuilder 创建 DELETE 构建器
func NewDeleteBuilder(table string) *DeleteBuilder {
	return &DeleteBuilder{
		table:    table,
		argIndex: 1,
	}
}

// Where 添加条件
func (b *DeleteBuilder) Where(clause string, args ...interface{}) *DeleteBuilder {
	var newClause strings.Builder
	for i := 0; i < len(clause); i++ {
		if clause[i] == '?' {
			newClause.WriteString("$")
			newClause.WriteString(strconv.Itoa(b.argIndex))
			b.argIndex++
		} else {
			newClause.WriteByte(clause[i])
		}
	}
	b.conditions = append(b.conditions, condition{
		clause: newClause.String(),
		args:   args,
	})
	return b
}

// Returning 设置返回列
func (b *DeleteBuilder) Returning(columns ...string) *DeleteBuilder {
	b.returning = columns
	return b
}

// Build 构建 DELETE 语句
func (b *DeleteBuilder) Build() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	sql.WriteString("DELETE FROM ")
	sql.WriteString(b.table)

	if len(b.conditions) > 0 {
		sql.WriteString(" WHERE ")
		clauses := make([]string, len(b.conditions))
		for i, cond := range b.conditions {
			clauses[i] = cond.clause
			args = append(args, cond.args...)
		}
		sql.WriteString(strings.Join(clauses, " AND "))
	}

	if len(b.returning) > 0 {
		sql.WriteString(" RETURNING ")
		sql.WriteString(strings.Join(b.returning, ", "))
	}

	return sql.String(), args
}

// QueryExecutor 查询执行器接口
type QueryExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// Execute 在执行器上执行构建的 SELECT 查询
func (b *Builder) Execute(ctx context.Context, executor QueryExecutor) (*sql.Rows, error) {
	query, args := b.BuildSelect()
	return executor.QueryContext(ctx, query, args...)
}

// ExecuteCount 在执行器上执行 COUNT 查询
func (b *Builder) ExecuteCount(ctx context.Context, executor QueryExecutor) (int64, error) {
	query, args := b.BuildCount()
	var count int64
	err := executor.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// Execute 在执行器上执行 INSERT 查询
func (b *InsertBuilder) Execute(ctx context.Context, executor QueryExecutor) (sql.Result, error) {
	query, args := b.Build()
	return executor.ExecContext(ctx, query, args...)
}

// ExecuteReturning 在执行器上执行 INSERT 并返回结果
func (b *InsertBuilder) ExecuteReturning(ctx context.Context, executor QueryExecutor) *sql.Row {
	query, args := b.Build()
	return executor.QueryRowContext(ctx, query, args...)
}

// Execute 在执行器上执行 UPDATE 查询
func (b *UpdateBuilder) Execute(ctx context.Context, executor QueryExecutor) (sql.Result, error) {
	query, args := b.Build()
	return executor.ExecContext(ctx, query, args...)
}

// ExecuteReturning 在执行器上执行 UPDATE 并返回结果
func (b *UpdateBuilder) ExecuteReturning(ctx context.Context, executor QueryExecutor) *sql.Row {
	query, args := b.Build()
	return executor.QueryRowContext(ctx, query, args...)
}

// Execute 在执行器上执行 DELETE 查询
func (b *DeleteBuilder) Execute(ctx context.Context, executor QueryExecutor) (sql.Result, error) {
	query, args := b.Build()
	return executor.ExecContext(ctx, query, args...)
}
