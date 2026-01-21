package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey struct{}
type traceIDKey struct{}
type userIDKey struct{}
type walletKey struct{}

var (
	globalLogger *zap.Logger
	sugarLogger  *zap.SugaredLogger
	atomicLevel  zap.AtomicLevel
	initOnce     sync.Once
)

// Config 日志配置
type Config struct {
	Level       string `yaml:"level" json:"level"`               // debug, info, warn, error
	Format      string `yaml:"format" json:"format"`             // json, console
	ServiceName string `yaml:"service_name" json:"service_name"` // 服务名称
	Environment string `yaml:"environment" json:"environment"`   // 环境: development, staging, production

	// 输出配置
	OutputPaths      []string `yaml:"output_paths" json:"output_paths"`             // 输出路径 (stdout, stderr, file paths)
	ErrorOutputPaths []string `yaml:"error_output_paths" json:"error_output_paths"` // 错误输出路径

	// 文件输出配置
	Filename   string `yaml:"filename" json:"filename"`       // 日志文件名
	MaxSize    int    `yaml:"max_size" json:"max_size"`       // 单个文件最大大小 (MB)
	MaxBackups int    `yaml:"max_backups" json:"max_backups"` // 最大备份数
	MaxAge     int    `yaml:"max_age" json:"max_age"`         // 最大保留天数
	Compress   bool   `yaml:"compress" json:"compress"`       // 是否压缩

	// 调用者信息
	AddCaller     bool `yaml:"add_caller" json:"add_caller"`         // 是否添加调用者信息
	CallerSkip    int  `yaml:"caller_skip" json:"caller_skip"`       // 调用者跳过层数
	AddStacktrace bool `yaml:"add_stacktrace" json:"add_stacktrace"` // 是否添加堆栈追踪

	// 采样配置
	EnableSampling bool `yaml:"enable_sampling" json:"enable_sampling"` // 是否启用采样
	SamplingInitial int  `yaml:"sampling_initial" json:"sampling_initial"` // 采样初始值
	SamplingAfter   int  `yaml:"sampling_after" json:"sampling_after"`     // 采样后续值
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Level:           "info",
		Format:          "json",
		ServiceName:     "unknown",
		Environment:     "development",
		AddCaller:       true,
		CallerSkip:      1,
		AddStacktrace:   false,
		EnableSampling:  false,
		SamplingInitial: 100,
		SamplingAfter:   100,
		MaxSize:         100,
		MaxBackups:      10,
		MaxAge:          30,
		Compress:        true,
	}
}

// Init 初始化全局日志
func Init(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	atomicLevel = zap.NewAtomicLevel()
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}
	atomicLevel.SetLevel(level)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if cfg.Format == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 构建输出
	var writers []zapcore.WriteSyncer
	writers = append(writers, zapcore.AddSync(os.Stdout))

	// 文件输出
	if cfg.Filename != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		writers = append(writers, zapcore.AddSync(fileWriter))
	}

	writeSyncer := zapcore.NewMultiWriteSyncer(writers...)

	// 构建核心
	core := zapcore.NewCore(encoder, writeSyncer, atomicLevel)

	// 采样
	if cfg.EnableSampling {
		core = zapcore.NewSamplerWithOptions(
			core,
			time.Second,
			cfg.SamplingInitial,
			cfg.SamplingAfter,
		)
	}

	// 构建选项
	opts := []zap.Option{
		zap.Fields(
			zap.String("service", cfg.ServiceName),
			zap.String("env", cfg.Environment),
		),
	}

	if cfg.AddCaller {
		opts = append(opts, zap.AddCaller())
		opts = append(opts, zap.AddCallerSkip(cfg.CallerSkip))
	}

	if cfg.AddStacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	globalLogger = zap.New(core, opts...)
	sugarLogger = globalLogger.Sugar()

	return nil
}

// InitOnce 只初始化一次
func InitOnce(cfg *Config) error {
	var err error
	initOnce.Do(func() {
		err = Init(cfg)
	})
	return err
}

// SetLevel 动态设置日志级别
func SetLevel(levelStr string) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		return
	}
	atomicLevel.SetLevel(level)
}

// L 获取全局 logger
func L() *zap.Logger {
	if globalLogger == nil {
		globalLogger, _ = zap.NewProduction()
	}
	return globalLogger
}

// S 获取全局 sugar logger
func S() *zap.SugaredLogger {
	if sugarLogger == nil {
		sugarLogger = L().Sugar()
	}
	return sugarLogger
}

// WithContext 从 context 获取带 trace_id 的 logger
func WithContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return L()
	}
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		return l
	}
	return L()
}

// NewContext 将 logger 放入 context
func NewContext(ctx context.Context, fields ...zap.Field) context.Context {
	return context.WithValue(ctx, ctxKey{}, L().With(fields...))
}

// Debug 调试日志
func Debug(msg string, fields ...zap.Field) {
	L().Debug(msg, fields...)
}

// Info 信息日志
func Info(msg string, fields ...zap.Field) {
	L().Info(msg, fields...)
}

// Warn 警告日志
func Warn(msg string, fields ...zap.Field) {
	L().Warn(msg, fields...)
}

// Error 错误日志
func Error(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
}

// Fatal 致命错误日志
func Fatal(msg string, fields ...zap.Field) {
	L().Fatal(msg, fields...)
}

// Sync 同步日志
func Sync() error {
	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}

// Named 创建带名称的子 logger
func Named(name string) *zap.Logger {
	return L().Named(name)
}

// With 创建带字段的子 logger
func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

// GetLevel 获取当前日志级别
func GetLevel() string {
	return atomicLevel.Level().String()
}

// IsDebugEnabled 检查是否启用 debug 级别
func IsDebugEnabled() bool {
	return atomicLevel.Level() <= zapcore.DebugLevel
}

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

// ContextLogger 从 context 创建 logger，自动添加 trace_id 等字段
func ContextLogger(ctx context.Context) *zap.Logger {
	l := L()

	if traceID := GetTraceID(ctx); traceID != "" {
		l = l.With(zap.String("trace_id", traceID))
	}
	if userID := GetUserID(ctx); userID != "" {
		l = l.With(zap.String("user_id", userID))
	}
	if wallet := GetWallet(ctx); wallet != "" {
		l = l.With(zap.String("wallet", wallet))
	}

	return l
}

// Debugf 格式化 debug 日志
func Debugf(format string, args ...interface{}) {
	S().Debugf(format, args...)
}

// Infof 格式化 info 日志
func Infof(format string, args ...interface{}) {
	S().Infof(format, args...)
}

// Warnf 格式化 warn 日志
func Warnf(format string, args ...interface{}) {
	S().Warnf(format, args...)
}

// Errorf 格式化 error 日志
func Errorf(format string, args ...interface{}) {
	S().Errorf(format, args...)
}

// Fatalf 格式化 fatal 日志
func Fatalf(format string, args ...interface{}) {
	S().Fatalf(format, args...)
}

// Panicf 格式化 panic 日志
func Panicf(format string, args ...interface{}) {
	S().Panicf(format, args...)
}

// LogWriter 实现 io.Writer 接口的日志写入器
type LogWriter struct {
	logger *zap.Logger
	level  zapcore.Level
}

// NewLogWriter 创建日志写入器
func NewLogWriter(level string) io.Writer {
	l := zapcore.InfoLevel
	_ = l.UnmarshalText([]byte(level))
	return &LogWriter{
		logger: L(),
		level:  l,
	}
}

// Write 实现 io.Writer 接口
func (w *LogWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	switch w.level {
	case zapcore.DebugLevel:
		w.logger.Debug(msg)
	case zapcore.InfoLevel:
		w.logger.Info(msg)
	case zapcore.WarnLevel:
		w.logger.Warn(msg)
	case zapcore.ErrorLevel:
		w.logger.Error(msg)
	default:
		w.logger.Info(msg)
	}
	return len(p), nil
}

// FileLogger 创建文件日志记录器
type FileLogger struct {
	logger   *zap.Logger
	filename string
	writer   *lumberjack.Logger
}

// NewFileLogger 创建文件日志记录器
func NewFileLogger(filename string, maxSize, maxBackups, maxAge int, compress bool) (*FileLogger, error) {
	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory failed: %w", err)
	}

	writer := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   compress,
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(writer),
		zap.DebugLevel,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return &FileLogger{
		logger:   logger,
		filename: filename,
		writer:   writer,
	}, nil
}

// Logger 返回 zap.Logger
func (f *FileLogger) Logger() *zap.Logger {
	return f.logger
}

// Close 关闭文件日志记录器
func (f *FileLogger) Close() error {
	_ = f.logger.Sync()
	return f.writer.Close()
}

// Rotate 手动轮转日志
func (f *FileLogger) Rotate() error {
	return f.writer.Rotate()
}

// AuditLog 审计日志
func AuditLog(ctx context.Context, action, resource string, details map[string]interface{}) {
	fields := []zap.Field{
		zap.String("audit_action", action),
		zap.String("audit_resource", resource),
		zap.Any("audit_details", details),
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	if userID := GetUserID(ctx); userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}
	if wallet := GetWallet(ctx); wallet != "" {
		fields = append(fields, zap.String("wallet", wallet))
	}

	L().Info("AUDIT", fields...)
}

// PerformanceLog 性能日志
func PerformanceLog(ctx context.Context, operation string, duration time.Duration, success bool) {
	fields := []zap.Field{
		zap.String("perf_operation", operation),
		zap.Duration("perf_duration", duration),
		zap.Bool("perf_success", success),
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}

	if duration > time.Second {
		L().Warn("SLOW_OPERATION", fields...)
	} else {
		L().Debug("PERFORMANCE", fields...)
	}
}
