// Package grpc 提供 gRPC 客户端工厂、服务器封装和拦截器
//
// 客户端使用:
//
//	factory := grpc.NewClientFactory(grpc.DefaultClientConfig())
//	conn, err := factory.GetConnection("localhost:9000")
//	if err != nil {
//		log.Fatal(err)
//	}
//	client := pb.NewMyServiceClient(conn)
//
// 服务器使用:
//
//	server, err := grpc.NewServer(grpc.DefaultServerConfig())
//	if err != nil {
//		log.Fatal(err)
//	}
//	pb.RegisterMyServiceServer(server.Server(), &myService{})
//	if err := server.Start(); err != nil {
//		log.Fatal(err)
//	}
//
// 自定义拦截器:
//
//	opts := []grpc.ServerOption{
//		grpc.ChainUnaryInterceptor(
//			grpc.UnaryServerTraceInterceptor(),
//			grpc.UnaryServerErrorInterceptor(),
//			grpc.TimeoutInterceptor(30 * time.Second),
//		),
//	}
package grpc

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// Version 包版本
const Version = "1.0.0"

// ContextKey 上下文键类型
type ContextKey string

// 上下文键常量
const (
	ContextKeyTraceID ContextKey = "trace_id"
	ContextKeyUserID  ContextKey = "user_id"
	ContextKeyWallet  ContextKey = "wallet"
)

// GetTraceID 从上下文获取追踪 ID
func GetTraceID(ctx context.Context) string {
	return extractFromContext(ctx, TraceIDKey)
}

// GetUserID 从上下文获取用户 ID
func GetUserID(ctx context.Context) string {
	return extractFromContext(ctx, UserIDKey)
}

// GetWallet 从上下文获取钱包地址
func GetWallet(ctx context.Context) string {
	return extractFromContext(ctx, WalletKey)
}

// SetOutgoingMetadata 设置 outgoing 元数据
func SetOutgoingMetadata(ctx context.Context, key, value string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, key, value)
}

// GetIncomingMetadata 获取 incoming 元数据
func GetIncomingMetadata(ctx context.Context, key string) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(key); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// GetOutgoingMetadata 获取 outgoing 元数据
func GetOutgoingMetadata(ctx context.Context, key string) string {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if values := md.Get(key); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
