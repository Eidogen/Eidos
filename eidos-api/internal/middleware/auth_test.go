package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestAuthMiddleware_MissingHeader 测试缺少认证头
func TestAuthMiddleware_MissingHeader_ReturnsError(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrMissingAuthHeader.Code, resp.Code)
}

// TestAuthMiddleware_InvalidFormat 测试无效的认证头格式
func TestAuthMiddleware_InvalidFormat_ReturnsError(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
	}{
		{"wrong_scheme", "Bearer token"},
		{"missing_parts", "EIP712 wallet:timestamp"},
		{"empty_payload", "EIP712 "},
		{"no_space", "EIP712wallet:123:sig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &AuthConfig{
				EIP712Config: &config.EIP712Config{
					MockMode:             true,
					TimestampToleranceMs: 60000,
				},
			}

			w := httptest.NewRecorder()
			c, r := gin.CreateTestContext(w)

			r.Use(Auth(cfg))
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
			c.Request.Header.Set(AuthHeader, tt.authHeader)
			r.ServeHTTP(w, c.Request)

			assert.Equal(t, http.StatusUnauthorized, w.Code)

			var resp dto.Response
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, dto.ErrInvalidAuthFormat.Code, resp.Code)
		})
	}
}

// TestAuthMiddleware_InvalidWalletAddress 测试无效的钱包地址
func TestAuthMiddleware_InvalidWalletAddress_ReturnsError(t *testing.T) {
	tests := []struct {
		name   string
		wallet string
	}{
		{"too_short", "0x123"},
		{"too_long", "0x" + string(make([]byte, 50))},
		{"no_prefix", "1234567890123456789012345678901234567890"},
		{"invalid_chars", "0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &AuthConfig{
				EIP712Config: &config.EIP712Config{
					MockMode:             true,
					TimestampToleranceMs: 60000,
				},
			}

			w := httptest.NewRecorder()
			c, r := gin.CreateTestContext(w)

			r.Use(Auth(cfg))
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			ts := time.Now().UnixMilli()
			authHeader := fmt.Sprintf("EIP712 %s:%d:0x1234", tt.wallet, ts)

			c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
			c.Request.Header.Set(AuthHeader, authHeader)
			r.ServeHTTP(w, c.Request)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp dto.Response
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, dto.ErrInvalidWalletAddr.Code, resp.Code)
		})
	}
}

// TestAuthMiddleware_ExpiredTimestamp 测试过期的时间戳
func TestAuthMiddleware_ExpiredTimestamp_ReturnsError(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000, // 1 minute tolerance
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 使用 2 分钟前的时间戳
	ts := time.Now().Add(-2 * time.Minute).UnixMilli()
	wallet := "0x1234567890123456789012345678901234567890"
	authHeader := fmt.Sprintf("EIP712 %s:%d:0x1234", wallet, ts)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrSignatureExpired.Code, resp.Code)
}

// TestAuthMiddleware_FutureTimestamp 测试未来的时间戳
func TestAuthMiddleware_FutureTimestamp_ReturnsError(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000, // 1 minute tolerance
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 使用 2 分钟后的时间戳
	ts := time.Now().Add(2 * time.Minute).UnixMilli()
	wallet := "0x1234567890123456789012345678901234567890"
	authHeader := fmt.Sprintf("EIP712 %s:%d:0x1234", wallet, ts)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrSignatureExpired.Code, resp.Code)
}

// TestAuthMiddleware_InvalidTimestamp 测试无效的时间戳格式
func TestAuthMiddleware_InvalidTimestamp_ReturnsError(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	wallet := "0x1234567890123456789012345678901234567890"
	authHeader := fmt.Sprintf("EIP712 %s:not-a-number:0x1234", wallet)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidTimestamp.Code, resp.Code)
}

// TestAuthMiddleware_MockMode_ValidRequest 测试 Mock 模式下的有效请求
func TestAuthMiddleware_MockMode_ValidRequest_Success(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	var capturedWallet string
	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		if wallet, exists := c.Get(WalletKey); exists {
			capturedWallet = wallet.(string)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	ts := time.Now().UnixMilli()
	wallet := "0x1234567890123456789012345678901234567890"
	authHeader := fmt.Sprintf("EIP712 %s:%d:0xabcdef", wallet, ts)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
	// 钱包地址应该被转换为小写
	assert.Equal(t, "0x1234567890123456789012345678901234567890", capturedWallet)
}

// TestAuthMiddleware_WalletAddressLowerCase 测试钱包地址转换为小写
func TestAuthMiddleware_WalletAddressLowerCase_Success(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	var capturedWallet string
	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		if wallet, exists := c.Get(WalletKey); exists {
			capturedWallet = wallet.(string)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	ts := time.Now().UnixMilli()
	// 使用大写字母的钱包地址
	wallet := "0xABCDEF1234567890ABCDEF1234567890ABCDEF12"
	authHeader := fmt.Sprintf("EIP712 %s:%d:0x1234", wallet, ts)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
	// 钱包地址应该被转换为小写
	assert.Equal(t, "0xabcdef1234567890abcdef1234567890abcdef12", capturedWallet)
}

// TestOptionalAuth_NoHeader 测试可选认证中间件无 Header 时放行
func TestOptionalAuth_NoHeader_PassThrough(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(OptionalAuth(cfg))
	r.GET("/test", func(c *gin.Context) {
		_, exists := c.Get(WalletKey)
		c.JSON(http.StatusOK, gin.H{"has_wallet": exists})
	})

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"has_wallet":false`)
}

// TestOptionalAuth_WithHeader 测试可选认证中间件有 Header 时验证
func TestOptionalAuth_WithHeader_Validates(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(OptionalAuth(cfg))
	r.GET("/test", func(c *gin.Context) {
		wallet, exists := c.Get(WalletKey)
		c.JSON(http.StatusOK, gin.H{"has_wallet": exists, "wallet": wallet})
	})

	ts := time.Now().UnixMilli()
	wallet := "0x1234567890123456789012345678901234567890"
	authHeader := fmt.Sprintf("EIP712 %s:%d:0x1234", wallet, ts)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"has_wallet":true`)
	assert.Contains(t, w.Body.String(), wallet)
}

// TestOptionalAuth_InvalidHeader 测试可选认证中间件有无效 Header 时返回错误
func TestOptionalAuth_InvalidHeader_ReturnsError(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 60000,
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(OptionalAuth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, "invalid")
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestParseAuthHeader 测试解析认证头
func TestParseAuthHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantWallet string
		wantTs    string
		wantSig   string
		wantErr   bool
	}{
		{
			name:       "valid_header",
			header:     "EIP712 0xwallet:12345:0xsignature",
			wantWallet: "0xwallet",
			wantTs:     "12345",
			wantSig:    "0xsignature",
			wantErr:    false,
		},
		{
			name:       "signature_with_colons",
			header:     "EIP712 0xwallet:12345:0x1234:5678:abcd",
			wantWallet: "0xwallet",
			wantTs:     "12345",
			wantSig:    "0x1234:5678:abcd",
			wantErr:    false,
		},
		{
			name:    "wrong_scheme",
			header:  "Bearer token",
			wantErr: true,
		},
		{
			name:    "missing_parts",
			header:  "EIP712 wallet:timestamp",
			wantErr: true,
		},
		{
			name:    "empty_payload",
			header:  "EIP712 ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet, ts, sig, err := parseAuthHeader(tt.header)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantWallet, wallet)
				assert.Equal(t, tt.wantTs, ts)
				assert.Equal(t, tt.wantSig, sig)
			}
		})
	}
}

// TestBuildSignMessage 测试构建签名消息
func TestBuildSignMessage(t *testing.T) {
	tests := []struct {
		method    string
		path      string
		timestamp int64
		want      string
	}{
		{"GET", "/api/v1/orders", 1704067200000, "GET:/api/v1/orders:1704067200000"},
		{"POST", "/api/v1/orders", 1704067200000, "POST:/api/v1/orders:1704067200000"},
		{"DELETE", "/api/v1/orders/123", 1704067200000, "DELETE:/api/v1/orders/123:1704067200000"},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			got := buildSignMessage(tt.method, tt.path, tt.timestamp)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDecodeSignature 测试解码签名
func TestDecodeSignature(t *testing.T) {
	tests := []struct {
		name    string
		sig     string
		want    []byte
		wantErr bool
	}{
		{
			name:    "with_0x_prefix",
			sig:     "0x1234abcd",
			want:    []byte{0x12, 0x34, 0xab, 0xcd},
			wantErr: false,
		},
		{
			name:    "without_0x_prefix",
			sig:     "1234abcd",
			want:    []byte{0x12, 0x34, 0xab, 0xcd},
			wantErr: false,
		},
		{
			name:    "invalid_hex",
			sig:     "0xghij",
			wantErr: true,
		},
		{
			name:    "odd_length",
			sig:     "0x123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSignature(tt.sig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestAuthMiddleware_NonMockMode_InvalidSignature 测试非 Mock 模式下签名验证失败
func TestAuthMiddleware_NonMockMode_InvalidSignature(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             false, // 非 Mock 模式
			TimestampToleranceMs: 60000,
			Domain: config.EIP712DomainConfig{
				Name:              "EidosExchange",
				Version:           "1",
				ChainID:           31337,
				VerifyingContract: "0x0000000000000000000000000000000000000000",
			},
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	ts := time.Now().UnixMilli()
	wallet := "0x1234567890123456789012345678901234567890"
	// 使用一个有效但不正确的签名（65 bytes = 130 hex chars）
	invalidSig := "0x" + "ab" + string(make([]byte, 128)) // 不是有效签名
	authHeader := fmt.Sprintf("EIP712 %s:%d:%s", wallet, ts, "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12")

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	// 签名验证应该失败
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidSignature.Code, resp.Code)

	_ = invalidSig // 避免未使用变量警告
}

// TestAuthMiddleware_NonMockMode_InvalidSignatureFormat 测试非 Mock 模式下无效签名格式
func TestAuthMiddleware_NonMockMode_InvalidSignatureFormat(t *testing.T) {
	cfg := &AuthConfig{
		EIP712Config: &config.EIP712Config{
			MockMode:             false, // 非 Mock 模式
			TimestampToleranceMs: 60000,
			Domain: config.EIP712DomainConfig{
				Name:              "EidosExchange",
				Version:           "1",
				ChainID:           31337,
				VerifyingContract: "0x0000000000000000000000000000000000000000",
			},
		},
	}

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(Auth(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	ts := time.Now().UnixMilli()
	wallet := "0x1234567890123456789012345678901234567890"
	// 使用无效的 hex 签名格式
	authHeader := fmt.Sprintf("EIP712 %s:%d:0xGGGGinvalid", wallet, ts)

	c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
	c.Request.Header.Set(AuthHeader, authHeader)
	r.ServeHTTP(w, c.Request)

	// 签名解码应该失败
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidSignature.Code, resp.Code)
}
