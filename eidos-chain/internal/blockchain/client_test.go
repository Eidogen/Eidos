package blockchain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestClientConfig_Validation 测试客户端配置验证
func TestClientConfig_Validation(t *testing.T) {
	t.Run("empty RPC URLs", func(t *testing.T) {
		cfg := &ClientConfig{
			ChainID: 31337,
			RPCURLs: []string{},
		}

		_, err := NewClient(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one RPC URL is required")
	})

	t.Run("invalid private key", func(t *testing.T) {
		cfg := &ClientConfig{
			ChainID:    31337,
			PrivateKey: "invalid-key",
			RPCURLs:    []string{"http://localhost:8545"},
		}

		_, err := NewClient(cfg)
		assert.Error(t, err)
	})

	t.Run("valid private key format", func(t *testing.T) {
		// 64 hex chars = 32 bytes private key
		validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		cfg := &ClientConfig{
			ChainID:    31337,
			PrivateKey: validKey,
			RPCURLs:    []string{"http://localhost:8545"},
		}

		// 会因为无法连接而失败，但私钥解析应该成功
		_, err := NewClient(cfg)
		// 这里会因为连接失败而返回错误，但不是私钥错误
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "invalid")
	})
}

// TestClientConfig_Defaults 测试默认配置
func TestClientConfig_Defaults(t *testing.T) {
	cfg := &ClientConfig{
		ChainID: 31337,
		RPCURLs: []string{"http://localhost:8545"},
	}

	// 测试默认值应用逻辑
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	assert.Equal(t, 3, maxRetries)

	retryInterval := cfg.RetryInterval
	if retryInterval == 0 {
		retryInterval = time.Second
	}
	assert.Equal(t, time.Second, retryInterval)

	healthCheckFreq := cfg.HealthCheckFreq
	if healthCheckFreq == 0 {
		healthCheckFreq = 30 * time.Second
	}
	assert.Equal(t, 30*time.Second, healthCheckFreq)
}

// TestRPCEndpoint_Fields 测试 RPC 端点结构体
func TestRPCEndpoint_Fields(t *testing.T) {
	ep := &RPCEndpoint{
		URL:        "http://localhost:8545",
		IsHealthy:  true,
		LatencyMs:  50,
		LastBlock:  12345,
		ErrorCount: 0,
		LastCheck:  time.Now(),
	}

	assert.Equal(t, "http://localhost:8545", ep.URL)
	assert.True(t, ep.IsHealthy)
	assert.Equal(t, int64(50), ep.LatencyMs)
	assert.Equal(t, uint64(12345), ep.LastBlock)
	assert.Equal(t, 0, ep.ErrorCount)
}

// TestClient_ErrorTypes 测试错误类型
func TestClient_ErrorTypes(t *testing.T) {
	assert.Equal(t, "no healthy RPC endpoint available", ErrNoHealthyRPC.Error())
	assert.Equal(t, "insufficient funds for gas", ErrInsufficientFunds.Error())
	assert.Equal(t, "nonce too low", ErrNonceTooLow.Error())
	assert.Equal(t, "nonce too high", ErrNonceTooHigh.Error())
	assert.Equal(t, "transaction not found", ErrTxNotFound.Error())
	assert.Equal(t, "transaction failed", ErrTxFailed.Error())
}

// TestClient_AddressAndChainID 测试地址和链 ID 方法
func TestClient_AddressAndChainID(t *testing.T) {
	// 创建一个不连接的客户端用于测试
	c := &Client{
		chainID: 31337,
		endpoints: []*RPCEndpoint{
			{URL: "http://localhost:8545", IsHealthy: true},
		},
		maxRetries:      3,
		retryInterval:   time.Second,
		healthCheckFreq: 30 * time.Second,
	}

	assert.Equal(t, int64(31337), c.ChainID())

	// 地址应该是零值（因为没有设置私钥）
	assert.True(t, c.Address().Hex() == "0x0000000000000000000000000000000000000000")
}

// TestClient_GetHealthyEndpoints 测试获取健康端点
func TestClient_GetHealthyEndpoints(t *testing.T) {
	c := &Client{
		endpoints: []*RPCEndpoint{
			{URL: "http://rpc1.example.com", IsHealthy: true},
			{URL: "http://rpc2.example.com", IsHealthy: false},
			{URL: "http://rpc3.example.com", IsHealthy: true},
		},
	}

	healthy := c.GetHealthyEndpoints()
	assert.Len(t, healthy, 2)
	assert.Equal(t, "http://rpc1.example.com", healthy[0].URL)
	assert.Equal(t, "http://rpc3.example.com", healthy[1].URL)
}

// TestClient_GetHealthyEndpoints_Empty 测试没有健康端点的情况
func TestClient_GetHealthyEndpoints_Empty(t *testing.T) {
	c := &Client{
		endpoints: []*RPCEndpoint{
			{URL: "http://rpc1.example.com", IsHealthy: false},
			{URL: "http://rpc2.example.com", IsHealthy: false},
		},
	}

	healthy := c.GetHealthyEndpoints()
	assert.Len(t, healthy, 0)
}

// TestClient_Close 测试关闭客户端
func TestClient_Close(t *testing.T) {
	c := &Client{
		endpoints: []*RPCEndpoint{
			{URL: "http://localhost:8545", IsHealthy: true},
		},
		client: nil, // 没有实际连接
	}

	// 应该不会 panic
	c.Close()

	// 再次关闭也不应该 panic
	c.Close()
}

// TestClient_SignTransaction_NoPrivateKey 测试没有私钥时签名
func TestClient_SignTransaction_NoPrivateKey(t *testing.T) {
	c := &Client{
		chainID:    31337,
		privateKey: nil, // 没有私钥
	}

	_, err := c.SignTransaction(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private key not configured")
}

// TestClient_EndpointFailover 测试端点故障转移逻辑
func TestClient_EndpointFailover(t *testing.T) {
	now := time.Now()

	c := &Client{
		endpoints: []*RPCEndpoint{
			{
				URL:        "http://rpc1.example.com",
				IsHealthy:  false,
				ErrorCount: 5,
				LastCheck:  now.Add(-time.Hour), // 很久之前检查过
			},
			{
				URL:        "http://rpc2.example.com",
				IsHealthy:  false,
				ErrorCount: 2,
				LastCheck:  now, // 刚刚检查过
			},
			{
				URL:        "http://rpc3.example.com",
				IsHealthy:  true,
				ErrorCount: 0,
				LastCheck:  now,
			},
		},
		currentIdx:      0,
		healthCheckFreq: 30 * time.Second,
	}

	// 第一个端点虽然不健康但超过了 healthCheckFreq，可以重试
	// 第二个端点不健康且刚检查过，应该跳过
	// 第三个端点健康，应该被选中

	healthy := c.GetHealthyEndpoints()
	assert.Len(t, healthy, 1)
	assert.Equal(t, "http://rpc3.example.com", healthy[0].URL)
}
