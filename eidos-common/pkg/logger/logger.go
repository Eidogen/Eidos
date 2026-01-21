// Package logger 提供统一的日志功能
// 基于 Go 1.21+ 标准库 slog 实现
package logger

import (
	"context"
	"log/slog"
	"os"
	"sync"
)

var (
	defaultLogger *slog.Logger
	once          sync.Once
)

// LogLevel 日志级别
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Config 日志配置
type Config struct {
	Level       string `yaml:"level" json:"level"`               // debug, info, warn, error
	Format      string `yaml:"format" json:"format"`             // json, text, color
	ServiceName string `yaml:"service_name" json:"service_name"` // 服务名称
	Environment string `yaml:"environment" json:"environment"`   // 环境: dev, staging, prod
}

// Init 初始化日志
func Init(cfg *Config) error {
	once.Do(func() {
		initLogger(cfg)
	})
	return nil
}

// initLogger 内部初始化函数
func initLogger(cfg *Config) {
	level := slog.LevelInfo
	if cfg != nil {
		switch LogLevel(cfg.Level) {
		case LevelDebug:
			level = slog.LevelDebug
		case LevelWarn:
			level = slog.LevelWarn
		case LevelError:
			level = slog.LevelError
		}
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	var handler slog.Handler
	format := "color"
	if cfg != nil && cfg.Format != "" {
		format = cfg.Format
	}

	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = NewColorHandler(os.Stdout, &ColorHandlerOptions{
			Level:     level,
			AddSource: opts.AddSource,
		})
	}

	if cfg != nil {
		var attrs []slog.Attr
		if cfg.ServiceName != "" {
			attrs = append(attrs, slog.String("service", cfg.ServiceName))
		}
		if cfg.Environment != "" {
			attrs = append(attrs, slog.String("env", cfg.Environment))
		}
		if len(attrs) > 0 {
			handler = handler.WithAttrs(attrs)
		}
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// Default 获取默认 logger
func Default() *slog.Logger {
	if defaultLogger == nil {
		Init(nil)
	}
	return defaultLogger
}

// L 获取默认 logger
func L() *slog.Logger {
	return Default()
}

// Debug 记录 debug 日志
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info 记录 info 日志
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn 记录 warn 日志
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error 记录 error 日志
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// Fatal 记录错误日志并退出程序
func Fatal(msg string, args ...any) {
	Default().Error(msg, args...)
	os.Exit(1)
}

// Sync 同步日志（slog 无需同步，保留接口兼容）
func Sync() error {
	return nil
}

// With 创建带有附加属性的 logger
func With(args ...any) *slog.Logger {
	return Default().With(args...)
}

// WithGroup 创建带有分组的 logger
func WithGroup(name string) *slog.Logger {
	return Default().WithGroup(name)
}

// Context key 类型
type traceIDKey struct{}
type userIDKey struct{}
type walletKey struct{}

// WithTraceID 创建带 trace_id 的 context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// GetTraceID 从 context 获取 trace_id
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// WithUserID 创建带 user_id 的 context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// GetUserID 从 context 获取 user_id
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey{}).(string); ok {
		return id
	}
	return ""
}

// WithWallet 创建带 wallet 的 context
func WithWallet(ctx context.Context, wallet string) context.Context {
	return context.WithValue(ctx, walletKey{}, wallet)
}

// GetWallet 从 context 获取 wallet
func GetWallet(ctx context.Context) string {
	if w, ok := ctx.Value(walletKey{}).(string); ok {
		return w
	}
	return ""
}

// ContextLogger 从 context 创建 logger
func ContextLogger(ctx context.Context) *slog.Logger {
	l := Default()
	var args []any
	if traceID := GetTraceID(ctx); traceID != "" {
		args = append(args, "trace_id", traceID)
	}
	if userID := GetUserID(ctx); userID != "" {
		args = append(args, "user_id", userID)
	}
	if wallet := GetWallet(ctx); wallet != "" {
		args = append(args, "wallet", wallet)
	}
	// 从 context 获取附加的日志属性
	if attrs := getContextAttrs(ctx); len(attrs) > 0 {
		for _, attr := range attrs {
			args = append(args, attr.Key, attr.Value.Any())
		}
	}
	if len(args) > 0 {
		return l.With(args...)
	}
	return l
}

// attrsKey 用于在 context 中存储日志属性
type attrsKey struct{}

// NewContext 创建带有日志属性的 context
func NewContext(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing := getContextAttrs(ctx)
	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)
	return context.WithValue(ctx, attrsKey{}, merged)
}

// getContextAttrs 从 context 获取日志属性
func getContextAttrs(ctx context.Context) []slog.Attr {
	if attrs, ok := ctx.Value(attrsKey{}).([]slog.Attr); ok {
		return attrs
	}
	return nil
}

// FromContext 从 context 获取 logger (带有 context 中的属性)
func FromContext(ctx context.Context) *slog.Logger {
	return ContextLogger(ctx)
}
