package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// ANSI 颜色码
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
	colorWhite   = "\033[97m"
)

// ColorHandler 彩色日志处理器
type ColorHandler struct {
	out       io.Writer
	level     slog.Level
	addSource bool
	attrs     []slog.Attr
	groups    []string
	mu        sync.Mutex
}

// ColorHandlerOptions 彩色处理器选项
type ColorHandlerOptions struct {
	Level     slog.Level
	AddSource bool
}

// NewColorHandler 创建彩色日志处理器
func NewColorHandler(out io.Writer, opts *ColorHandlerOptions) *ColorHandler {
	if out == nil {
		out = os.Stdout
	}
	if opts == nil {
		opts = &ColorHandlerOptions{Level: slog.LevelInfo}
	}
	return &ColorHandler{
		out:       out,
		level:     opts.Level,
		addSource: opts.AddSource,
	}
}

// Enabled 检查是否启用指定级别
func (h *ColorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle 处理日志记录
func (h *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 时间格式：ISO 8601，精确到毫秒
	timeStr := r.Time.Format("2006-01-02T15:04:05.000Z07:00")

	// 级别颜色
	levelColor, levelStr := h.levelColor(r.Level)

	// 构建输出
	fmt.Fprintf(h.out, "%s%s%s %s%-5s%s %s%s%s",
		colorGray, timeStr, colorReset,
		levelColor, levelStr, colorReset,
		colorWhite, r.Message, colorReset,
	)

	// 输出预设属性
	for _, attr := range h.attrs {
		h.writeAttr(attr)
	}

	// 输出记录属性
	r.Attrs(func(a slog.Attr) bool {
		h.writeAttr(a)
		return true
	})

	fmt.Fprintln(h.out)
	return nil
}

// WithAttrs 添加属性
func (h *ColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := &ColorHandler{
		out:       h.out,
		level:     h.level,
		addSource: h.addSource,
		attrs:     make([]slog.Attr, len(h.attrs)+len(attrs)),
		groups:    h.groups,
	}
	copy(newHandler.attrs, h.attrs)
	copy(newHandler.attrs[len(h.attrs):], attrs)
	return newHandler
}

// WithGroup 添加分组
func (h *ColorHandler) WithGroup(name string) slog.Handler {
	newHandler := &ColorHandler{
		out:       h.out,
		level:     h.level,
		addSource: h.addSource,
		attrs:     h.attrs,
		groups:    append(h.groups, name),
	}
	return newHandler
}

// levelColor 返回级别颜色和文本
func (h *ColorHandler) levelColor(level slog.Level) (string, string) {
	switch {
	case level >= slog.LevelError:
		return colorRed + colorBold, "ERROR"
	case level >= slog.LevelWarn:
		return colorYellow, "WARN "
	case level >= slog.LevelInfo:
		return colorGreen, "INFO "
	default:
		return colorBlue, "DEBUG"
	}
}

// writeAttr 输出属性
func (h *ColorHandler) writeAttr(a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}

	key := a.Key
	val := a.Value.Any()

	// 根据键名选择颜色
	valColor := h.attrValueColor(key, val)

	// 格式化输出
	fmt.Fprintf(h.out, " %s%s%s=%s%v%s",
		colorMagenta, key, colorReset,
		valColor, h.formatValue(val), colorReset,
	)
}

// attrValueColor 根据属性键名和值选择颜色
func (h *ColorHandler) attrValueColor(key string, val any) string {
	// 错误字段
	if key == "error" || key == "err" {
		return colorRed
	}

	// 追踪字段
	if key == "trace_id" || key == "request_id" || key == "span_id" {
		return colorCyan
	}

	// 耗时字段
	if key == "latency" || key == "duration" || key == "took" || key == "elapsed" || key == "elapsed_ms" {
		return colorYellow
	}

	// 状态码
	if key == "status" || key == "status_code" {
		if code, ok := val.(int); ok {
			if code >= 500 {
				return colorRed
			} else if code >= 400 {
				return colorYellow
			} else if code >= 200 && code < 300 {
				return colorGreen
			}
		}
	}

	// 服务名
	if key == "service" || key == "component" {
		return colorCyan
	}

	return colorWhite
}

// formatValue 格式化值
func (h *ColorHandler) formatValue(val any) string {
	switch v := val.(type) {
	case time.Duration:
		return v.String()
	case time.Time:
		return v.Format(time.RFC3339)
	case error:
		return v.Error()
	case string:
		if len(v) > 100 {
			return v[:100] + "..."
		}
		return v
	default:
		return fmt.Sprintf("%v", val)
	}
}
