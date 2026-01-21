package middleware

import (
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eidos-exchange/eidos/eidos-api/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/crypto"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/gin-gonic/gin"
)

const (
	// AuthScheme EIP-712 认证方案
	AuthScheme = "EIP712"
	// AuthHeader 认证头名称
	AuthHeader = "Authorization"
	// WalletKey context 中的钱包地址键名
	WalletKey = "wallet"
)

// 钱包地址正则
var walletAddrRegex = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)

// AuthConfig 认证中间件配置
type AuthConfig struct {
	EIP712Config *config.EIP712Config
	ReplayGuard  *cache.ReplayGuard
}

// Auth 返回 EIP-712 认证中间件
func Auth(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 解析 Authorization 头
		authHeader := c.GetHeader(AuthHeader)
		if authHeader == "" {
			abortWithError(c, dto.ErrMissingAuthHeader)
			return
		}

		// 格式: EIP712 {wallet}:{timestamp}:{signature}
		wallet, timestamp, signature, err := parseAuthHeader(authHeader)
		if err != nil {
			abortWithError(c, dto.ErrInvalidAuthFormat)
			return
		}

		// 2. 验证钱包地址格式
		if !walletAddrRegex.MatchString(wallet) {
			abortWithError(c, dto.ErrInvalidWalletAddr)
			return
		}

		// 3. 验证时间戳
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			abortWithError(c, dto.ErrInvalidTimestamp)
			return
		}

		now := time.Now().UnixMilli()
		tolerance := cfg.EIP712Config.TimestampToleranceMs
		if now-ts > tolerance || ts-now > tolerance {
			logger.Warn("signature expired",
				"wallet", wallet,
				"timestamp", ts,
				"now", now,
				"diff", now-ts,
			)
			abortWithError(c, dto.ErrSignatureExpired)
			return
		}

		// 4. 检查重放攻击
		if cfg.ReplayGuard != nil {
			allowed, err := cfg.ReplayGuard.CheckAndMark(c.Request.Context(), wallet, timestamp, signature)
			if err != nil {
				logger.Error("replay guard check failed", "error", err)
				// 出错时仍然继续，不阻塞请求
			} else if !allowed {
				logger.Warn("signature replay detected",
					"wallet", wallet,
					"timestamp", timestamp,
				)
				abortWithError(c, dto.ErrSignatureReplay)
				return
			}
		}

		// 5. 验证签名（Mock 模式跳过实际验证）
		if !cfg.EIP712Config.MockMode {
			// 构建签名消息: method + path + timestamp
			message := buildSignMessage(c.Request.Method, c.Request.URL.Path, ts)

			// 解码签名
			sigBytes, err := decodeSignature(signature)
			if err != nil {
				abortWithError(c, dto.ErrInvalidSignature)
				return
			}

			// 从配置构建 EIP-712 Domain
			domain := crypto.EIP712Domain{
				Name:              cfg.EIP712Config.Domain.Name,
				Version:           cfg.EIP712Config.Domain.Version,
				ChainID:           cfg.EIP712Config.Domain.ChainID,
				VerifyingContract: cfg.EIP712Config.Domain.VerifyingContract,
			}

			// 计算消息哈希
			messageHash := crypto.Keccak256([]byte(message))

			// 计算 EIP-712 最终哈希
			typedHash := crypto.HashTypedDataV4(domain, messageHash)

			// 验证签名
			valid, err := crypto.VerifySignature(wallet, typedHash, sigBytes)
			if err != nil || !valid {
				logger.Warn("signature verification failed",
					"wallet", wallet,
					"error", err,
				)
				abortWithError(c, dto.ErrInvalidSignature)
				return
			}
		}

		// 6. 设置 wallet 到 context
		c.Set(WalletKey, strings.ToLower(wallet))

		c.Next()
	}
}

// parseAuthHeader 解析认证头
// 格式: EIP712 {wallet}:{timestamp}:{signature}
func parseAuthHeader(header string) (wallet, timestamp, signature string, err error) {
	// 检查前缀
	if !strings.HasPrefix(header, AuthScheme+" ") {
		return "", "", "", dto.ErrInvalidAuthFormat
	}

	// 去掉前缀
	payload := strings.TrimPrefix(header, AuthScheme+" ")

	// 分割
	parts := strings.SplitN(payload, ":", 3)
	if len(parts) != 3 {
		return "", "", "", dto.ErrInvalidAuthFormat
	}

	return parts[0], parts[1], parts[2], nil
}

// buildSignMessage 构建签名消息
func buildSignMessage(method, path string, timestamp int64) string {
	return method + ":" + path + ":" + strconv.FormatInt(timestamp, 10)
}

// decodeSignature 解码签名
func decodeSignature(sig string) ([]byte, error) {
	// 去掉 0x 前缀
	sig = strings.TrimPrefix(sig, "0x")
	return hex.DecodeString(sig)
}

// abortWithError 终止请求并返回错误
func abortWithError(c *gin.Context, err *dto.BizError) {
	c.AbortWithStatusJSON(err.HTTPStatus, dto.NewErrorResponse(err))
}

// OptionalAuth 返回可选认证中间件（如果有认证头则验证，没有则跳过）
func OptionalAuth(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthHeader)
		if authHeader == "" {
			c.Next()
			return
		}

		// 有认证头则走认证流程
		Auth(cfg)(c)
	}
}
