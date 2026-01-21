// Package contract provides smart contract related utilities.
package contract

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// Token registry errors
var (
	ErrTokenNotFound      = errors.New("token not found")
	ErrInvalidTokenConfig = errors.New("invalid token configuration")
	ErrChainNotSupported  = errors.New("chain not supported")
)

// ERC20ABI is the minimal ABI for ERC20 token queries.
const ERC20ABI = `[
	{
		"type": "function",
		"name": "symbol",
		"inputs": [],
		"outputs": [{"name": "", "type": "string"}],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "decimals",
		"inputs": [],
		"outputs": [{"name": "", "type": "uint8"}],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "name",
		"inputs": [],
		"outputs": [{"name": "", "type": "string"}],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "balanceOf",
		"inputs": [{"name": "account", "type": "address"}],
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "totalSupply",
		"inputs": [],
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view"
	}
]`

// TokenInfo represents information about a token.
type TokenInfo struct {
	Symbol   string         `json:"symbol"`
	Name     string         `json:"name"`
	Address  common.Address `json:"address"`
	Decimals uint8          `json:"decimals"`
	ChainID  int64          `json:"chain_id"`
	IsNative bool           `json:"is_native"`
}

// TokenRegistryConfig is the configuration for the token registry.
type TokenRegistryConfig struct {
	ChainID int64
	// Predefined tokens (address -> TokenInfo)
	Tokens map[string]*TokenInfo
}

// TokenRegistry manages the mapping between token addresses and symbols.
type TokenRegistry struct {
	mu sync.RWMutex

	chainID int64

	// address -> TokenInfo
	tokensByAddress map[common.Address]*TokenInfo
	// symbol -> TokenInfo
	tokensBySymbol map[string]*TokenInfo

	// ERC20 ABI for on-chain queries
	erc20ABI abi.ABI

	// Contract caller for on-chain queries
	caller bind.ContractCaller
}

// NewTokenRegistry creates a new token registry.
func NewTokenRegistry(cfg *TokenRegistryConfig, caller bind.ContractCaller) (*TokenRegistry, error) {
	parsed, err := abi.JSON(strings.NewReader(ERC20ABI))
	if err != nil {
		return nil, err
	}

	r := &TokenRegistry{
		chainID:         cfg.ChainID,
		tokensByAddress: make(map[common.Address]*TokenInfo),
		tokensBySymbol:  make(map[string]*TokenInfo),
		erc20ABI:        parsed,
		caller:          caller,
	}

	// Register native token (ETH)
	nativeToken := &TokenInfo{
		Symbol:   "ETH",
		Name:     "Ethereum",
		Address:  NativeToken(),
		Decimals: 18,
		ChainID:  cfg.ChainID,
		IsNative: true,
	}
	r.tokensByAddress[NativeToken()] = nativeToken
	r.tokensBySymbol["ETH"] = nativeToken

	// Register predefined tokens
	if cfg.Tokens != nil {
		for addrStr, info := range cfg.Tokens {
			addr := common.HexToAddress(addrStr)
			info.Address = addr
			info.ChainID = cfg.ChainID
			r.tokensByAddress[addr] = info
			r.tokensBySymbol[info.Symbol] = info
		}
	}

	return r, nil
}

// RegisterToken registers a new token.
func (r *TokenRegistry) RegisterToken(info *TokenInfo) error {
	if info == nil {
		return ErrInvalidTokenConfig
	}
	if info.Symbol == "" {
		return errors.New("token symbol is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info.ChainID = r.chainID
	r.tokensByAddress[info.Address] = info
	r.tokensBySymbol[info.Symbol] = info

	return nil
}

// GetByAddress returns the token info for a given address.
func (r *TokenRegistry) GetByAddress(address common.Address) (*TokenInfo, error) {
	r.mu.RLock()
	info, ok := r.tokensByAddress[address]
	r.mu.RUnlock()

	if ok {
		return info, nil
	}

	return nil, ErrTokenNotFound
}

// GetBySymbol returns the token info for a given symbol.
func (r *TokenRegistry) GetBySymbol(symbol string) (*TokenInfo, error) {
	r.mu.RLock()
	info, ok := r.tokensBySymbol[strings.ToUpper(symbol)]
	r.mu.RUnlock()

	if ok {
		return info, nil
	}

	return nil, ErrTokenNotFound
}

// GetSymbol returns the symbol for a token address.
// Falls back to on-chain query if not cached.
func (r *TokenRegistry) GetSymbol(ctx context.Context, address common.Address) (string, error) {
	// Check cache first
	r.mu.RLock()
	info, ok := r.tokensByAddress[address]
	r.mu.RUnlock()

	if ok {
		return info.Symbol, nil
	}

	// Query on-chain
	symbol, err := r.querySymbol(ctx, address)
	if err != nil {
		return "", err
	}

	// Fetch additional info and cache
	go r.fetchAndCacheToken(context.Background(), address)

	return symbol, nil
}

// GetDecimals returns the decimals for a token address.
func (r *TokenRegistry) GetDecimals(ctx context.Context, address common.Address) (uint8, error) {
	// Check cache first
	r.mu.RLock()
	info, ok := r.tokensByAddress[address]
	r.mu.RUnlock()

	if ok {
		return info.Decimals, nil
	}

	// Query on-chain
	decimals, err := r.queryDecimals(ctx, address)
	if err != nil {
		return 0, err
	}

	return decimals, nil
}

// GetAddress returns the address for a token symbol.
func (r *TokenRegistry) GetAddress(symbol string) (common.Address, error) {
	r.mu.RLock()
	info, ok := r.tokensBySymbol[strings.ToUpper(symbol)]
	r.mu.RUnlock()

	if ok {
		return info.Address, nil
	}

	return common.Address{}, ErrTokenNotFound
}

// GetAllTokens returns all registered tokens.
func (r *TokenRegistry) GetAllTokens() []*TokenInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tokens := make([]*TokenInfo, 0, len(r.tokensByAddress))
	for _, info := range r.tokensByAddress {
		tokens = append(tokens, info)
	}

	return tokens
}

// GetAllSymbols returns all registered token symbols.
func (r *TokenRegistry) GetAllSymbols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	symbols := make([]string, 0, len(r.tokensBySymbol))
	for symbol := range r.tokensBySymbol {
		symbols = append(symbols, symbol)
	}

	return symbols
}

// BalanceOf queries the balance of a token for an account.
func (r *TokenRegistry) BalanceOf(ctx context.Context, token, account common.Address) (*big.Int, error) {
	// Native token
	if IsNativeToken(token) {
		if caller, ok := r.caller.(interface {
			BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
		}); ok {
			return caller.BalanceAt(ctx, account, nil)
		}
		return nil, errors.New("caller does not support BalanceAt")
	}

	// ERC20 token
	data, err := r.erc20ABI.Pack("balanceOf", account)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &token,
		Data: data,
	}

	result, err := r.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	var balance *big.Int
	err = r.erc20ABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

// querySymbol queries the symbol of a token from the blockchain.
func (r *TokenRegistry) querySymbol(ctx context.Context, address common.Address) (string, error) {
	if r.caller == nil {
		return "", errors.New("no contract caller configured")
	}

	// Handle native token
	if IsNativeToken(address) {
		return "ETH", nil
	}

	data, err := r.erc20ABI.Pack("symbol")
	if err != nil {
		return "", err
	}

	msg := ethereum.CallMsg{
		To:   &address,
		Data: data,
	}

	result, err := r.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return "", err
	}

	var symbol string
	err = r.erc20ABI.UnpackIntoInterface(&symbol, "symbol", result)
	if err != nil {
		return "", err
	}

	return symbol, nil
}

// queryDecimals queries the decimals of a token from the blockchain.
func (r *TokenRegistry) queryDecimals(ctx context.Context, address common.Address) (uint8, error) {
	if r.caller == nil {
		return 0, errors.New("no contract caller configured")
	}

	// Handle native token
	if IsNativeToken(address) {
		return 18, nil
	}

	data, err := r.erc20ABI.Pack("decimals")
	if err != nil {
		return 0, err
	}

	msg := ethereum.CallMsg{
		To:   &address,
		Data: data,
	}

	result, err := r.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return 0, err
	}

	var decimals uint8
	err = r.erc20ABI.UnpackIntoInterface(&decimals, "decimals", result)
	if err != nil {
		return 0, err
	}

	return decimals, nil
}

// queryName queries the name of a token from the blockchain.
func (r *TokenRegistry) queryName(ctx context.Context, address common.Address) (string, error) {
	if r.caller == nil {
		return "", errors.New("no contract caller configured")
	}

	// Handle native token
	if IsNativeToken(address) {
		return "Ethereum", nil
	}

	data, err := r.erc20ABI.Pack("name")
	if err != nil {
		return "", err
	}

	msg := ethereum.CallMsg{
		To:   &address,
		Data: data,
	}

	result, err := r.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return "", err
	}

	var name string
	err = r.erc20ABI.UnpackIntoInterface(&name, "name", result)
	if err != nil {
		return "", err
	}

	return name, nil
}

// fetchAndCacheToken fetches token info from the blockchain and caches it.
func (r *TokenRegistry) fetchAndCacheToken(ctx context.Context, address common.Address) {
	symbol, err := r.querySymbol(ctx, address)
	if err != nil {
		return
	}

	decimals, err := r.queryDecimals(ctx, address)
	if err != nil {
		decimals = 18 // Default to 18 decimals
	}

	name, _ := r.queryName(ctx, address) // Name is optional

	info := &TokenInfo{
		Symbol:   symbol,
		Name:     name,
		Address:  address,
		Decimals: decimals,
		ChainID:  r.chainID,
		IsNative: false,
	}

	r.mu.Lock()
	r.tokensByAddress[address] = info
	r.tokensBySymbol[symbol] = info
	r.mu.Unlock()
}

// DefaultTokens returns the default token configuration for common networks.
func DefaultTokens(chainID int64) map[string]*TokenInfo {
	switch chainID {
	case 1: // Ethereum Mainnet
		return map[string]*TokenInfo{
			"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48": {Symbol: "USDC", Name: "USD Coin", Decimals: 6},
			"0xdAC17F958D2ee523a2206206994597C13D831ec7": {Symbol: "USDT", Name: "Tether USD", Decimals: 6},
			"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2": {Symbol: "WETH", Name: "Wrapped Ether", Decimals: 18},
			"0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599": {Symbol: "WBTC", Name: "Wrapped BTC", Decimals: 8},
		}
	case 42161: // Arbitrum One
		return map[string]*TokenInfo{
			"0xaf88d065e77c8cC2239327C5EDb3A432268e5831": {Symbol: "USDC", Name: "USD Coin", Decimals: 6},
			"0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9": {Symbol: "USDT", Name: "Tether USD", Decimals: 6},
			"0x82aF49447D8a07e3bd95BD0d56f35241523fBab1": {Symbol: "WETH", Name: "Wrapped Ether", Decimals: 18},
			"0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f": {Symbol: "WBTC", Name: "Wrapped BTC", Decimals: 8},
		}
	case 421614: // Arbitrum Sepolia
		return map[string]*TokenInfo{
			"0x75faf114eafb1BDbe2F0316DF893fd58CE46AA4d": {Symbol: "USDC", Name: "USD Coin", Decimals: 6},
		}
	case 31337: // Local development
		return map[string]*TokenInfo{
			"0x5FbDB2315678afecb367f032d93F642f64180aa3": {Symbol: "USDC", Name: "USD Coin", Decimals: 6},
			"0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512": {Symbol: "USDT", Name: "Tether USD", Decimals: 6},
		}
	default:
		return map[string]*TokenInfo{}
	}
}
