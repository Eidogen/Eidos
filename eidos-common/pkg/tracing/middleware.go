package tracing

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GinMiddleware 返回 Gin 链路追踪中间件
func GinMiddleware(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		// 从请求头提取 trace context
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		// 创建 span
		spanName := c.FullPath()
		if spanName == "" {
			spanName = c.Request.URL.Path
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(c.Request.Method),
				semconv.HTTPRoute(spanName),
				semconv.HTTPScheme(c.Request.URL.Scheme),
				semconv.HTTPTarget(c.Request.URL.Path),
				semconv.NetHostName(c.Request.Host),
				semconv.UserAgentOriginal(c.Request.UserAgent()),
			),
		)
		defer span.End()

		// 设置 trace ID 到响应头
		if span.SpanContext().HasTraceID() {
			c.Header("X-Trace-ID", span.SpanContext().TraceID().String())
		}

		// 将 context 传递给后续处理
		c.Request = c.Request.WithContext(ctx)

		// 处理请求
		c.Next()

		// 记录响应状态
		statusCode := c.Writer.Status()
		span.SetAttributes(semconv.HTTPStatusCode(statusCode))

		// 记录错误
		if len(c.Errors) > 0 {
			span.RecordError(c.Errors.Last().Err)
			span.SetStatus(codes.Error, c.Errors.Last().Error())
		} else if statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		}
	}
}

// UnaryServerInterceptor 返回 gRPC unary 服务端拦截器
func UnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 从 metadata 提取 trace context
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = propagator.Extract(ctx, metadataCarrier(md))
		}

		// 创建 span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceName),
				semconv.RPCMethod(info.FullMethod),
			),
		)
		defer span.End()

		// 处理请求
		resp, err := handler(ctx, req)

		// 记录状态
		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.Int("grpc.status_code", int(st.Code())))
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		} else {
			span.SetAttributes(attribute.Int("grpc.status_code", 0))
		}

		return resp, err
	}
}

// StreamServerInterceptor 返回 gRPC stream 服务端拦截器
func StreamServerInterceptor(serviceName string) grpc.StreamServerInterceptor {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		// 从 metadata 提取 trace context
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = propagator.Extract(ctx, metadataCarrier(md))
		}

		// 创建 span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceName),
				semconv.RPCMethod(info.FullMethod),
				attribute.Bool("grpc.stream.client", info.IsClientStream),
				attribute.Bool("grpc.stream.server", info.IsServerStream),
			),
		)
		defer span.End()

		// 包装 stream 以传递 context
		wrappedStream := &tracingServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// 处理请求
		err := handler(srv, wrappedStream)

		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.Int("grpc.status_code", int(st.Code())))
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		}

		return err
	}
}

// UnaryClientInterceptor 返回 gRPC unary 客户端拦截器
func UnaryClientInterceptor(serviceName string) grpc.UnaryClientInterceptor {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// 创建 span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceName),
				semconv.RPCMethod(method),
			),
		)
		defer span.End()

		// 注入 trace context 到 metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		propagator.Inject(ctx, metadataCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		// 调用
		err := invoker(ctx, method, req, reply, cc, opts...)

		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.Int("grpc.status_code", int(st.Code())))
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		} else {
			span.SetAttributes(attribute.Int("grpc.status_code", 0))
		}

		return err
	}
}

// StreamClientInterceptor 返回 gRPC stream 客户端拦截器
func StreamClientInterceptor(serviceName string) grpc.StreamClientInterceptor {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		// 创建 span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(cc.Target()),
				semconv.RPCMethod(method),
				attribute.Bool("grpc.stream.client", desc.ClientStreams),
				attribute.Bool("grpc.stream.server", desc.ServerStreams),
			),
		)

		// 注入 trace context 到 metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		propagator.Inject(ctx, metadataCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		// 调用
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return nil, err
		}

		return &tracingClientStream{
			ClientStream: cs,
			span:         span,
		}, nil
	}
}

// metadataCarrier 实现 TextMapCarrier 接口
type metadataCarrier metadata.MD

func (m metadataCarrier) Get(key string) string {
	values := metadata.MD(m).Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func (m metadataCarrier) Set(key, value string) {
	metadata.MD(m).Set(key, value)
}

func (m metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// tracingServerStream 包装 ServerStream 以传递 context
type tracingServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *tracingServerStream) Context() context.Context {
	return s.ctx
}

// tracingClientStream 包装 ClientStream 以在关闭时结束 span
type tracingClientStream struct {
	grpc.ClientStream
	span trace.Span
}

func (s *tracingClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End()
	return err
}
