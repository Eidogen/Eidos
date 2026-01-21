// Package middleware 提供 gRPC 服务端和客户端的通用中间件
//
// 服务端中间件使用:
//
//	server := grpc.NewServer(
//		grpc.ChainUnaryInterceptor(
//			middleware.RecoveryUnaryServerInterceptor(),
//			middleware.UnaryServerInterceptor(),
//			middleware.ValidatorUnaryServerInterceptor(),
//			middleware.TimeoutUnaryServerInterceptor(30 * time.Second),
//		),
//		grpc.ChainStreamInterceptor(
//			middleware.RecoveryStreamServerInterceptor(),
//			middleware.StreamServerInterceptor(),
//		),
//	)
//
// 客户端中间件使用:
//
//	conn, err := grpc.Dial(target,
//		grpc.WithChainUnaryInterceptor(
//			middleware.UnaryClientInterceptor(),
//			middleware.MetricsUnaryClientInterceptor("myservice"),
//		),
//	)
//
// 认证中间件使用:
//
//	authenticator := middleware.NewSignatureAuthenticator(verifier)
//	server := grpc.NewServer(
//		grpc.ChainUnaryInterceptor(
//			middleware.AuthUnaryServerInterceptor(authenticator, &middleware.AuthConfig{
//				SkipMethods: []string{"/health.HealthService/Check"},
//			}),
//		),
//	)
//
// 限流中间件使用:
//
//	limiter := middleware.NewTokenBucketLimiter(1000, 100) // 每秒 1000 请求，桶容量 100
//	server := grpc.NewServer(
//		grpc.ChainUnaryInterceptor(
//			middleware.RateLimitUnaryServerInterceptor(limiter),
//		),
//	)
package middleware

import (
	"google.golang.org/grpc"
)

// Version 包版本
const Version = "1.0.0"

// DefaultUnaryServerInterceptors 返回默认的服务端一元拦截器链
func DefaultUnaryServerInterceptors() []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		RecoveryUnaryServerInterceptor(),
		UnaryServerInterceptor(),
		ValidatorUnaryServerInterceptor(),
	}
}

// DefaultStreamServerInterceptors 返回默认的服务端流拦截器链
func DefaultStreamServerInterceptors() []grpc.StreamServerInterceptor {
	return []grpc.StreamServerInterceptor{
		RecoveryStreamServerInterceptor(),
		StreamServerInterceptor(),
	}
}

// DefaultUnaryClientInterceptors 返回默认的客户端一元拦截器链
func DefaultUnaryClientInterceptors() []grpc.UnaryClientInterceptor {
	return []grpc.UnaryClientInterceptor{
		UnaryClientInterceptor(),
	}
}

// ServerOptions 返回推荐的服务端选项
func ServerOptions(serviceName string) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			RecoveryUnaryServerInterceptor(),
			UnaryServerInterceptor(),
			MetricsUnaryServerInterceptor(serviceName),
			ValidatorUnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			RecoveryStreamServerInterceptor(),
			StreamServerInterceptor(),
			MetricsStreamServerInterceptor(serviceName),
		),
	}
}

// ClientOptions 返回推荐的客户端选项
func ClientOptions(serviceName string) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(
			UnaryClientInterceptor(),
			MetricsUnaryClientInterceptor(serviceName),
		),
		grpc.WithChainStreamInterceptor(
			MetricsStreamClientInterceptor(serviceName),
		),
	}
}
