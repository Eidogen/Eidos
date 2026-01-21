package middleware

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthConfig 认证配置
type AuthConfig struct {
	// 跳过认证的方法列表
	SkipMethods []string
	// 是否允许匿名访问
	AllowAnonymous bool
}

// DefaultAuthConfig 默认认证配置
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		SkipMethods:    []string{},
		AllowAnonymous: false,
	}
}

// Authenticator 认证器接口
type Authenticator interface {
	// Authenticate 认证请求，返回用户信息或错误
	Authenticate(ctx context.Context) (context.Context, error)
}

// UserInfo 用户信息
type UserInfo struct {
	UserID   string
	Wallet   string
	Roles    []string
	Metadata map[string]string
}

// userInfoKey 用户信息上下文键
type userInfoKey struct{}

// GetUserInfo 从上下文获取用户信息
func GetUserInfo(ctx context.Context) (*UserInfo, bool) {
	info, ok := ctx.Value(userInfoKey{}).(*UserInfo)
	return info, ok
}

// SetUserInfo 将用户信息放入上下文
func SetUserInfo(ctx context.Context, info *UserInfo) context.Context {
	return context.WithValue(ctx, userInfoKey{}, info)
}

// SignatureAuthenticator 签名认证器
type SignatureAuthenticator struct {
	verifier SignatureVerifier
}

// SignatureVerifier 签名验证器接口
type SignatureVerifier interface {
	// Verify 验证签名，返回钱包地址
	Verify(ctx context.Context, signature, message string) (string, error)
}

// NewSignatureAuthenticator 创建签名认证器
func NewSignatureAuthenticator(verifier SignatureVerifier) *SignatureAuthenticator {
	return &SignatureAuthenticator{verifier: verifier}
}

// Authenticate 认证
func (a *SignatureAuthenticator) Authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	// 获取签名和消息
	signatures := md.Get("x-signature")
	messages := md.Get("x-message")

	if len(signatures) == 0 || len(messages) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing signature or message")
	}

	// 验证签名
	wallet, err := a.verifier.Verify(ctx, signatures[0], messages[0])
	if err != nil {
		logger.Debug("signature verification failed",
			"error", err,
		)
		return nil, status.Error(codes.Unauthenticated, "invalid signature")
	}

	// 设置用户信息
	userInfo := &UserInfo{
		Wallet: wallet,
	}

	return SetUserInfo(ctx, userInfo), nil
}

// APIKeyAuthenticator API Key 认证器
type APIKeyAuthenticator struct {
	validator APIKeyValidator
}

// APIKeyValidator API Key 验证器接口
type APIKeyValidator interface {
	// Validate 验证 API Key，返回用户信息
	Validate(ctx context.Context, apiKey string) (*UserInfo, error)
}

// NewAPIKeyAuthenticator 创建 API Key 认证器
func NewAPIKeyAuthenticator(validator APIKeyValidator) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{validator: validator}
}

// Authenticate 认证
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	// 获取 API Key
	apiKeys := md.Get("x-api-key")
	if len(apiKeys) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing API key")
	}

	// 验证 API Key
	userInfo, err := a.validator.Validate(ctx, apiKeys[0])
	if err != nil {
		logger.Debug("API key validation failed",
			"error", err,
		)
		return nil, status.Error(codes.Unauthenticated, "invalid API key")
	}

	return SetUserInfo(ctx, userInfo), nil
}

// CompositeAuthenticator 组合认证器
type CompositeAuthenticator struct {
	authenticators []Authenticator
}

// NewCompositeAuthenticator 创建组合认证器
func NewCompositeAuthenticator(authenticators ...Authenticator) *CompositeAuthenticator {
	return &CompositeAuthenticator{authenticators: authenticators}
}

// Authenticate 认证 (尝试所有认证器，任一成功即可)
func (a *CompositeAuthenticator) Authenticate(ctx context.Context) (context.Context, error) {
	var lastErr error

	for _, auth := range a.authenticators {
		newCtx, err := auth.Authenticate(ctx)
		if err == nil {
			return newCtx, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, status.Error(codes.Unauthenticated, "authentication failed")
}

// AuthUnaryServerInterceptor 认证拦截器
func AuthUnaryServerInterceptor(auth Authenticator, cfg *AuthConfig) grpc.UnaryServerInterceptor {
	if cfg == nil {
		cfg = DefaultAuthConfig()
	}

	skipMethods := make(map[string]bool)
	for _, m := range cfg.SkipMethods {
		skipMethods[m] = true
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 检查是否跳过认证
		if skipMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// 执行认证
		newCtx, err := auth.Authenticate(ctx)
		if err != nil {
			if cfg.AllowAnonymous {
				return handler(ctx, req)
			}
			return nil, err
		}

		return handler(newCtx, req)
	}
}

// AuthStreamServerInterceptor 流式认证拦截器
func AuthStreamServerInterceptor(auth Authenticator, cfg *AuthConfig) grpc.StreamServerInterceptor {
	if cfg == nil {
		cfg = DefaultAuthConfig()
	}

	skipMethods := make(map[string]bool)
	for _, m := range cfg.SkipMethods {
		skipMethods[m] = true
	}

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// 检查是否跳过认证
		if skipMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		ctx := ss.Context()

		// 执行认证
		newCtx, err := auth.Authenticate(ctx)
		if err != nil {
			if cfg.AllowAnonymous {
				return handler(srv, ss)
			}
			return err
		}

		// 包装 stream 以使用新的 context
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          newCtx,
		}

		return handler(srv, wrapped)
	}
}

// wrappedServerStream 包装的服务器流
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
