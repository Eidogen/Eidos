// Package tracing 提供 OpenTelemetry 链路追踪支持
// 使用 Jaeger 作为后端存储
package tracing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config 链路追踪配置
type Config struct {
	// Enabled 是否启用链路追踪
	Enabled bool `yaml:"enabled" json:"enabled"`
	// ServiceName 服务名称
	ServiceName string `yaml:"service_name" json:"service_name"`
	// Endpoint OTLP 端点地址 (如 localhost:4317)
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	// SampleRate 采样率 (0.0 - 1.0)
	SampleRate float64 `yaml:"sample_rate" json:"sample_rate"`
	// Environment 环境 (dev, staging, prod)
	Environment string `yaml:"environment" json:"environment"`
	// Version 服务版本
	Version string `yaml:"version" json:"version"`
	// Insecure 是否使用不安全连接
	Insecure bool `yaml:"insecure" json:"insecure"`
	// Timeout 连接超时
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:     false,
		ServiceName: "eidos-service",
		Endpoint:    "localhost:4317",
		SampleRate:  1.0,
		Environment: "dev",
		Version:     "1.0.0",
		Insecure:    true,
		Timeout:     5 * time.Second,
	}
}

// TracerProvider 全局追踪器提供者
var globalTracerProvider *sdktrace.TracerProvider

// Init 初始化链路追踪
func Init(cfg *Config) (func(context.Context) error, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if !cfg.Enabled {
		// 返回空的关闭函数
		return func(ctx context.Context) error { return nil }, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// 创建 OTLP exporter
	// 处理 endpoint 格式：移除 http:// 或 https:// 前缀
	endpoint := cfg.Endpoint
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter failed: %w", err)
	}

	// 创建资源 (仅使用显式属性，避免 Schema URL 冲突)
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.Version),
		semconv.DeploymentEnvironment(cfg.Environment),
	)

	// 创建采样器
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// 创建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)
	globalTracerProvider = tp

	// 设置全局传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 返回关闭函数
	return tp.Shutdown, nil
}

// Tracer 获取追踪器
func Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return otel.Tracer(name, opts...)
}

// StartSpan 开始一个新的 Span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("").Start(ctx, name, opts...)
}

// StartSpanWithTracer 使用指定追踪器开始一个新的 Span
func StartSpanWithTracer(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, spanName, opts...)
}

// SpanFromContext 从 context 获取当前 Span
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent 添加事件到当前 Span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes 设置当前 Span 的属性
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// RecordError 记录错误到当前 Span
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	trace.SpanFromContext(ctx).RecordError(err, opts...)
}

// SetStatus 设置 Span 状态
func SetStatus(ctx context.Context, code codes.Code, description string) {
	trace.SpanFromContext(ctx).SetStatus(code, description)
}

// TraceID 获取当前 Trace ID
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// SpanID 获取当前 Span ID
func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// IsRecording 检查当前 Span 是否正在记录
func IsRecording(ctx context.Context) bool {
	return trace.SpanFromContext(ctx).IsRecording()
}

// 常用属性键
var (
	// Service 属性
	AttrServiceName    = semconv.ServiceNameKey
	AttrServiceVersion = semconv.ServiceVersionKey

	// HTTP 属性
	AttrHTTPMethod     = semconv.HTTPMethodKey
	AttrHTTPURL        = semconv.HTTPURLKey
	AttrHTTPStatusCode = semconv.HTTPStatusCodeKey
	AttrHTTPRoute      = semconv.HTTPRouteKey

	// gRPC 属性
	AttrRPCMethod  = semconv.RPCMethodKey
	AttrRPCService = semconv.RPCServiceKey

	// Database 属性
	AttrDBSystem    = semconv.DBSystemKey
	AttrDBStatement = semconv.DBStatementKey
	AttrDBOperation = semconv.DBOperationKey

	// Messaging 属性
	AttrMessagingSystem      = semconv.MessagingSystemKey
	AttrMessagingDestination = semconv.MessagingDestinationNameKey
	AttrMessagingOperation   = semconv.MessagingOperationKey
)

// 业务属性键
const (
	AttrOrderID     = attribute.Key("order.id")
	AttrUserID      = attribute.Key("user.id")
	AttrMarket      = attribute.Key("market.symbol")
	AttrOrderSide   = attribute.Key("order.side")
	AttrOrderType   = attribute.Key("order.type")
	AttrOrderPrice  = attribute.Key("order.price")
	AttrOrderAmount = attribute.Key("order.amount")
	AttrTradeID     = attribute.Key("trade.id")
	AttrTxHash      = attribute.Key("tx.hash")
)
