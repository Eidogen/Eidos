// Package contract provides smart contract related utilities.
package contract

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// Gas estimation errors
var (
	ErrGasEstimationFailed = errors.New("gas estimation failed")
	ErrGasPriceTooHigh     = errors.New("gas price exceeds maximum")
	ErrGasLimitTooHigh     = errors.New("gas limit exceeds maximum")
)

// GasEstimatorConfig is the configuration for the gas estimator.
type GasEstimatorConfig struct {
	// MaxGasPrice is the maximum gas price in wei.
	MaxGasPrice *big.Int
	// MaxGasLimit is the maximum gas limit.
	MaxGasLimit uint64
	// GasPriceMultiplier is the multiplier for suggested gas price (1.1 = 10% buffer).
	GasPriceMultiplier float64
	// GasLimitMultiplier is the multiplier for estimated gas (1.2 = 20% buffer).
	GasLimitMultiplier float64
	// CacheTTL is the time-to-live for cached gas prices.
	CacheTTL time.Duration
	// BaseGasForSettlement is the base gas for settlement transactions.
	BaseGasForSettlement uint64
	// GasPerTrade is the additional gas per trade in settlement.
	GasPerTrade uint64
	// BaseGasForWithdrawal is the base gas for withdrawal transactions.
	BaseGasForWithdrawal uint64
}

// GasPriceInfo contains gas price information.
type GasPriceInfo struct {
	// Legacy gas price (for non-EIP-1559 chains).
	GasPrice *big.Int
	// EIP-1559 fields.
	BaseFee   *big.Int
	GasTipCap *big.Int // Max priority fee per gas.
	GasFeeCap *big.Int // Max fee per gas.
	// Timestamp when this info was fetched.
	FetchedAt time.Time
}

// GasEstimate contains the result of gas estimation.
type GasEstimate struct {
	// Estimated gas limit.
	GasLimit uint64
	// Gas price info.
	GasPrice *GasPriceInfo
	// Estimated total cost in wei.
	EstimatedCost *big.Int
	// Whether EIP-1559 is used.
	IsEIP1559 bool
}

// GasEstimator estimates gas for transactions.
type GasEstimator struct {
	cfg *GasEstimatorConfig

	// Backend for RPC calls
	backend interface {
		SuggestGasPrice(ctx context.Context) (*big.Int, error)
		SuggestGasTipCap(ctx context.Context) (*big.Int, error)
		EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
		HeaderByNumber(ctx context.Context, number *big.Int) (interface{ BaseFee() *big.Int }, error)
	}

	// Cache
	mu           sync.RWMutex
	cachedPrice  *GasPriceInfo
	isEIP1559    bool
	eip1559Check time.Time
}

// EthBackend interface for Ethereum client.
type EthBackend interface {
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (interface{ BaseFee() *big.Int }, error)
}

// NewGasEstimator creates a new gas estimator.
func NewGasEstimator(cfg *GasEstimatorConfig, backend EthBackend) *GasEstimator {
	if cfg == nil {
		cfg = &GasEstimatorConfig{}
	}

	// Set defaults
	if cfg.MaxGasPrice == nil {
		cfg.MaxGasPrice = big.NewInt(500e9) // 500 Gwei
	}
	if cfg.MaxGasLimit == 0 {
		cfg.MaxGasLimit = 10_000_000
	}
	if cfg.GasPriceMultiplier == 0 {
		cfg.GasPriceMultiplier = 1.1
	}
	if cfg.GasLimitMultiplier == 0 {
		cfg.GasLimitMultiplier = 1.2
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 12 * time.Second // ~1 block on Ethereum
	}
	if cfg.BaseGasForSettlement == 0 {
		cfg.BaseGasForSettlement = 100_000
	}
	if cfg.GasPerTrade == 0 {
		cfg.GasPerTrade = 50_000
	}
	if cfg.BaseGasForWithdrawal == 0 {
		cfg.BaseGasForWithdrawal = 80_000
	}

	return &GasEstimator{
		cfg:     cfg,
		backend: backend,
	}
}

// GetGasPrice returns the current gas price information.
func (e *GasEstimator) GetGasPrice(ctx context.Context) (*GasPriceInfo, error) {
	// Check cache
	e.mu.RLock()
	if e.cachedPrice != nil && time.Since(e.cachedPrice.FetchedAt) < e.cfg.CacheTTL {
		cached := e.cachedPrice
		e.mu.RUnlock()
		return cached, nil
	}
	e.mu.RUnlock()

	// Fetch fresh gas price
	return e.fetchGasPrice(ctx)
}

// fetchGasPrice fetches fresh gas price from the network.
func (e *GasEstimator) fetchGasPrice(ctx context.Context) (*GasPriceInfo, error) {
	info := &GasPriceInfo{
		FetchedAt: time.Now(),
	}

	// Try EIP-1559 first
	gasTipCap, err := e.backend.SuggestGasTipCap(ctx)
	if err == nil && gasTipCap != nil && gasTipCap.Sign() > 0 {
		// EIP-1559 supported
		info.GasTipCap = gasTipCap

		// Get base fee from latest block header if available
		if headerFetcher, ok := e.backend.(interface {
			HeaderByNumber(ctx context.Context, number *big.Int) (interface{ BaseFee() *big.Int }, error)
		}); ok {
			header, err := headerFetcher.HeaderByNumber(ctx, nil)
			if err == nil && header != nil {
				info.BaseFee = header.BaseFee()
			}
		}

		// Calculate gas fee cap (2 * base fee + tip)
		if info.BaseFee != nil {
			info.GasFeeCap = new(big.Int).Mul(info.BaseFee, big.NewInt(2))
			info.GasFeeCap.Add(info.GasFeeCap, info.GasTipCap)
		} else {
			// Fallback: use suggested gas price as fee cap
			gasPrice, _ := e.backend.SuggestGasPrice(ctx)
			info.GasFeeCap = gasPrice
		}
	}

	// Always get legacy gas price as fallback
	gasPrice, err := e.backend.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	info.GasPrice = gasPrice

	// Apply multiplier
	if e.cfg.GasPriceMultiplier > 1 {
		multiplied := new(big.Float).SetInt(info.GasPrice)
		multiplied.Mul(multiplied, big.NewFloat(e.cfg.GasPriceMultiplier))
		info.GasPrice, _ = multiplied.Int(nil)

		if info.GasFeeCap != nil {
			multiplied = new(big.Float).SetInt(info.GasFeeCap)
			multiplied.Mul(multiplied, big.NewFloat(e.cfg.GasPriceMultiplier))
			info.GasFeeCap, _ = multiplied.Int(nil)
		}
	}

	// Check against maximum
	if info.GasPrice.Cmp(e.cfg.MaxGasPrice) > 0 {
		return nil, ErrGasPriceTooHigh
	}

	// Cache the result
	e.mu.Lock()
	e.cachedPrice = info
	e.isEIP1559 = info.GasTipCap != nil && info.GasTipCap.Sign() > 0
	e.mu.Unlock()

	return info, nil
}

// EstimateSettlementGas estimates gas for a settlement transaction.
func (e *GasEstimator) EstimateSettlementGas(ctx context.Context, from, to common.Address, data []byte, tradeCount int) (*GasEstimate, error) {
	// Try actual estimation first
	gasLimit, err := e.estimateGas(ctx, from, to, data)
	if err != nil {
		// Fall back to formula-based estimation
		gasLimit = e.cfg.BaseGasForSettlement + uint64(tradeCount)*e.cfg.GasPerTrade
	}

	// Apply multiplier for safety margin
	gasLimit = uint64(float64(gasLimit) * e.cfg.GasLimitMultiplier)

	// Check against maximum
	if gasLimit > e.cfg.MaxGasLimit {
		return nil, ErrGasLimitTooHigh
	}

	// Get gas price
	gasPrice, err := e.GetGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate estimated cost
	estimatedCost := new(big.Int).Mul(gasPrice.GasPrice, big.NewInt(int64(gasLimit)))

	return &GasEstimate{
		GasLimit:      gasLimit,
		GasPrice:      gasPrice,
		EstimatedCost: estimatedCost,
		IsEIP1559:     e.isEIP1559,
	}, nil
}

// EstimateWithdrawalGas estimates gas for a withdrawal transaction.
func (e *GasEstimator) EstimateWithdrawalGas(ctx context.Context, from, to common.Address, data []byte) (*GasEstimate, error) {
	// Try actual estimation first
	gasLimit, err := e.estimateGas(ctx, from, to, data)
	if err != nil {
		// Fall back to formula-based estimation
		gasLimit = e.cfg.BaseGasForWithdrawal
	}

	// Apply multiplier for safety margin
	gasLimit = uint64(float64(gasLimit) * e.cfg.GasLimitMultiplier)

	// Check against maximum
	if gasLimit > e.cfg.MaxGasLimit {
		return nil, ErrGasLimitTooHigh
	}

	// Get gas price
	gasPrice, err := e.GetGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate estimated cost
	estimatedCost := new(big.Int).Mul(gasPrice.GasPrice, big.NewInt(int64(gasLimit)))

	return &GasEstimate{
		GasLimit:      gasLimit,
		GasPrice:      gasPrice,
		EstimatedCost: estimatedCost,
		IsEIP1559:     e.isEIP1559,
	}, nil
}

// estimateGas performs the actual gas estimation via RPC.
func (e *GasEstimator) estimateGas(ctx context.Context, from, to common.Address, data []byte) (uint64, error) {
	msg := ethereum.CallMsg{
		From: from,
		To:   &to,
		Data: data,
	}

	gas, err := e.backend.EstimateGas(ctx, msg)
	if err != nil {
		return 0, err
	}

	return gas, nil
}

// EstimateGas estimates gas for any transaction.
func (e *GasEstimator) EstimateGas(ctx context.Context, from, to common.Address, data []byte, value *big.Int) (*GasEstimate, error) {
	msg := ethereum.CallMsg{
		From:  from,
		To:    &to,
		Data:  data,
		Value: value,
	}

	gasLimit, err := e.backend.EstimateGas(ctx, msg)
	if err != nil {
		return nil, err
	}

	// Apply multiplier for safety margin
	gasLimit = uint64(float64(gasLimit) * e.cfg.GasLimitMultiplier)

	// Check against maximum
	if gasLimit > e.cfg.MaxGasLimit {
		return nil, ErrGasLimitTooHigh
	}

	// Get gas price
	gasPrice, err := e.GetGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate estimated cost
	estimatedCost := new(big.Int).Mul(gasPrice.GasPrice, big.NewInt(int64(gasLimit)))

	return &GasEstimate{
		GasLimit:      gasLimit,
		GasPrice:      gasPrice,
		EstimatedCost: estimatedCost,
		IsEIP1559:     e.isEIP1559,
	}, nil
}

// IsEIP1559Supported returns whether EIP-1559 is supported.
func (e *GasEstimator) IsEIP1559Supported() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isEIP1559
}

// InvalidateCache invalidates the cached gas price.
func (e *GasEstimator) InvalidateCache() {
	e.mu.Lock()
	e.cachedPrice = nil
	e.mu.Unlock()
}

// SetMaxGasPrice updates the maximum gas price.
func (e *GasEstimator) SetMaxGasPrice(maxGasPrice *big.Int) {
	e.cfg.MaxGasPrice = maxGasPrice
}

// SetMaxGasLimit updates the maximum gas limit.
func (e *GasEstimator) SetMaxGasLimit(maxGasLimit uint64) {
	e.cfg.MaxGasLimit = maxGasLimit
}
