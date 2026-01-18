package blockchain

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	ErrNoHealthyRPC      = errors.New("no healthy RPC endpoint available")
	ErrInsufficientFunds = errors.New("insufficient funds for gas")
	ErrNonceTooLow       = errors.New("nonce too low")
	ErrNonceTooHigh      = errors.New("nonce too high")
	ErrTxNotFound        = errors.New("transaction not found")
	ErrTxFailed          = errors.New("transaction failed")
)

// RPCEndpoint RPC 端点信息
type RPCEndpoint struct {
	URL        string
	IsHealthy  bool
	LatencyMs  int64
	LastBlock  uint64
	ErrorCount int
	LastCheck  time.Time
}

// Client 区块链客户端
type Client struct {
	chainID    int64
	privateKey *ecdsa.PrivateKey
	address    common.Address

	endpoints  []*RPCEndpoint
	currentIdx int
	mu         sync.RWMutex

	client *ethclient.Client

	// 配置
	maxRetries      int
	retryInterval   time.Duration
	healthCheckFreq time.Duration
}

// ClientConfig 客户端配置
type ClientConfig struct {
	ChainID         int64
	PrivateKey      string
	RPCURLs         []string
	MaxRetries      int
	RetryInterval   time.Duration
	HealthCheckFreq time.Duration
}

// NewClient 创建区块链客户端
func NewClient(cfg *ClientConfig) (*Client, error) {
	if len(cfg.RPCURLs) == 0 {
		return nil, errors.New("at least one RPC URL is required")
	}

	var privateKey *ecdsa.PrivateKey
	var address common.Address

	if cfg.PrivateKey != "" {
		var err error
		privateKey, err = crypto.HexToECDSA(cfg.PrivateKey)
		if err != nil {
			return nil, err
		}
		address = crypto.PubkeyToAddress(privateKey.PublicKey)
	}

	endpoints := make([]*RPCEndpoint, len(cfg.RPCURLs))
	for i, url := range cfg.RPCURLs {
		endpoints[i] = &RPCEndpoint{
			URL:       url,
			IsHealthy: true,
		}
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryInterval := cfg.RetryInterval
	if retryInterval == 0 {
		retryInterval = time.Second
	}

	healthCheckFreq := cfg.HealthCheckFreq
	if healthCheckFreq == 0 {
		healthCheckFreq = 30 * time.Second
	}

	c := &Client{
		chainID:         cfg.ChainID,
		privateKey:      privateKey,
		address:         address,
		endpoints:       endpoints,
		maxRetries:      maxRetries,
		retryInterval:   retryInterval,
		healthCheckFreq: healthCheckFreq,
	}

	// 连接到第一个可用的 RPC
	if err := c.connect(context.Background()); err != nil {
		return nil, err
	}

	return c, nil
}

// connect 连接到可用的 RPC
func (c *Client) connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.endpoints {
		idx := (c.currentIdx + i) % len(c.endpoints)
		ep := c.endpoints[idx]

		if !ep.IsHealthy && time.Since(ep.LastCheck) < c.healthCheckFreq {
			continue
		}

		client, err := ethclient.DialContext(ctx, ep.URL)
		if err != nil {
			ep.IsHealthy = false
			ep.ErrorCount++
			ep.LastCheck = time.Now()
			continue
		}

		// 检查连接
		_, err = client.ChainID(ctx)
		if err != nil {
			client.Close()
			ep.IsHealthy = false
			ep.ErrorCount++
			ep.LastCheck = time.Now()
			continue
		}

		if c.client != nil {
			c.client.Close()
		}

		c.client = client
		c.currentIdx = idx
		ep.IsHealthy = true
		ep.ErrorCount = 0
		ep.LastCheck = time.Now()
		return nil
	}

	return ErrNoHealthyRPC
}

// getClient 获取客户端，如果不可用则尝试重连
func (c *Client) getClient(ctx context.Context) (*ethclient.Client, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client != nil {
		return client, nil
	}

	if err := c.connect(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client, nil
}

// withRetry 带重试的操作
func (c *Client) withRetry(ctx context.Context, fn func(*ethclient.Client) error) error {
	var lastErr error
	for i := 0; i < c.maxRetries; i++ {
		client, err := c.getClient(ctx)
		if err != nil {
			lastErr = err
			time.Sleep(c.retryInterval)
			continue
		}

		err = fn(client)
		if err == nil {
			return nil
		}

		lastErr = err

		// 标记当前端点为不健康
		c.mu.Lock()
		if c.currentIdx < len(c.endpoints) {
			c.endpoints[c.currentIdx].IsHealthy = false
			c.endpoints[c.currentIdx].ErrorCount++
		}
		c.mu.Unlock()

		// 尝试重连
		if i < c.maxRetries-1 {
			c.connect(ctx)
			time.Sleep(c.retryInterval)
		}
	}
	return lastErr
}

// Address 返回热钱包地址
func (c *Client) Address() common.Address {
	return c.address
}

// ChainID 返回链 ID
func (c *Client) ChainID() int64 {
	return c.chainID
}

// BlockNumber 获取最新区块号
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	var blockNum uint64
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		blockNum, err = client.BlockNumber(ctx)
		return err
	})
	return blockNum, err
}

// GetBlockByNumber 获取区块
func (c *Client) GetBlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	var block *types.Block
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		block, err = client.BlockByNumber(ctx, number)
		return err
	})
	return block, err
}

// GetTransactionReceipt 获取交易回执
func (c *Client) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		receipt, err = client.TransactionReceipt(ctx, txHash)
		if err == ethereum.NotFound {
			return ErrTxNotFound
		}
		return err
	})
	return receipt, err
}

// PendingNonceAt 获取待处理 Nonce
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	var nonce uint64
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		nonce, err = client.PendingNonceAt(ctx, account)
		return err
	})
	return nonce, err
}

// SuggestGasPrice 获取建议 Gas 价格
func (c *Client) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	var gasPrice *big.Int
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		gasPrice, err = client.SuggestGasPrice(ctx)
		return err
	})
	return gasPrice, err
}

// SuggestGasTipCap 获取建议 Gas Tip (EIP-1559)
func (c *Client) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	var gasTip *big.Int
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		gasTip, err = client.SuggestGasTipCap(ctx)
		return err
	})
	return gasTip, err
}

// EstimateGas 估算 Gas
func (c *Client) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	var gas uint64
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		gas, err = client.EstimateGas(ctx, msg)
		return err
	})
	return gas, err
}

// SendTransaction 发送交易
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.withRetry(ctx, func(client *ethclient.Client) error {
		return client.SendTransaction(ctx, tx)
	})
}

// FilterLogs 过滤日志
func (c *Client) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	var logs []types.Log
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		logs, err = client.FilterLogs(ctx, query)
		return err
	})
	return logs, err
}

// BalanceAt 获取余额
func (c *Client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	var balance *big.Int
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		balance, err = client.BalanceAt(ctx, account, blockNumber)
		return err
	})
	return balance, err
}

// CallContract 调用合约
func (c *Client) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var result []byte
	err := c.withRetry(ctx, func(client *ethclient.Client) error {
		var err error
		result, err = client.CallContract(ctx, msg, blockNumber)
		return err
	})
	return result, err
}

// SignTransaction 签名交易
func (c *Client) SignTransaction(tx *types.Transaction) (*types.Transaction, error) {
	if c.privateKey == nil {
		return nil, errors.New("private key not configured")
	}

	signer := types.NewEIP155Signer(big.NewInt(c.chainID))
	return types.SignTx(tx, signer, c.privateKey)
}

// Close 关闭客户端
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.BlockNumber(ctx)
	return err
}

// GetHealthyEndpoints 获取健康的端点列表
func (c *Client) GetHealthyEndpoints() []*RPCEndpoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var healthy []*RPCEndpoint
	for _, ep := range c.endpoints {
		if ep.IsHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy
}
