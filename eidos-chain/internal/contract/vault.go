// Package contract provides smart contract ABI bindings for the Eidos Exchange.
package contract

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Vault contract errors
var (
	ErrVaultContractNotDeployed = errors.New("vault contract not deployed")
	ErrInvalidWithdrawalData    = errors.New("invalid withdrawal data")
	ErrInsufficientVaultBalance = errors.New("insufficient vault balance")
	ErrWithdrawalNotFound       = errors.New("withdrawal not found")
)

// VaultABI is the ABI of the Vault smart contract.
// This matches the Solidity contract interface:
//
//	function deposit(address token, uint256 amount) external;
//	function withdraw(address user, address token, uint256 amount, address to, uint256 nonce, bytes signature) external;
//	function getBalance(address user, address token) external view returns (uint256);
//	event Deposit(address indexed user, address indexed token, uint256 amount);
//	event Withdraw(address indexed user, address indexed token, uint256 amount, address to);
const VaultABI = `[
	{
		"type": "function",
		"name": "deposit",
		"inputs": [
			{"name": "token", "type": "address"},
			{"name": "amount", "type": "uint256"}
		],
		"outputs": [],
		"stateMutability": "payable"
	},
	{
		"type": "function",
		"name": "withdraw",
		"inputs": [
			{"name": "user", "type": "address"},
			{"name": "token", "type": "address"},
			{"name": "amount", "type": "uint256"},
			{"name": "to", "type": "address"},
			{"name": "nonce", "type": "uint256"},
			{"name": "signature", "type": "bytes"}
		],
		"outputs": [
			{"name": "success", "type": "bool"}
		],
		"stateMutability": "nonpayable"
	},
	{
		"type": "function",
		"name": "batchWithdraw",
		"inputs": [
			{
				"name": "withdrawals",
				"type": "tuple[]",
				"components": [
					{"name": "user", "type": "address"},
					{"name": "token", "type": "address"},
					{"name": "amount", "type": "uint256"},
					{"name": "to", "type": "address"},
					{"name": "nonce", "type": "uint256"},
					{"name": "signature", "type": "bytes"}
				]
			}
		],
		"outputs": [
			{"name": "results", "type": "bool[]"}
		],
		"stateMutability": "nonpayable"
	},
	{
		"type": "function",
		"name": "getBalance",
		"inputs": [
			{"name": "user", "type": "address"},
			{"name": "token", "type": "address"}
		],
		"outputs": [
			{"name": "balance", "type": "uint256"}
		],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "getNonce",
		"inputs": [
			{"name": "user", "type": "address"}
		],
		"outputs": [
			{"name": "nonce", "type": "uint256"}
		],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "isWithdrawalProcessed",
		"inputs": [
			{"name": "user", "type": "address"},
			{"name": "nonce", "type": "uint256"}
		],
		"outputs": [
			{"name": "processed", "type": "bool"}
		],
		"stateMutability": "view"
	},
	{
		"type": "event",
		"name": "Deposit",
		"inputs": [
			{"name": "user", "type": "address", "indexed": true},
			{"name": "token", "type": "address", "indexed": true},
			{"name": "amount", "type": "uint256", "indexed": false}
		]
	},
	{
		"type": "event",
		"name": "Withdraw",
		"inputs": [
			{"name": "user", "type": "address", "indexed": true},
			{"name": "token", "type": "address", "indexed": true},
			{"name": "amount", "type": "uint256", "indexed": false},
			{"name": "to", "type": "address", "indexed": false},
			{"name": "nonce", "type": "uint256", "indexed": false}
		]
	},
	{
		"type": "event",
		"name": "WithdrawFailed",
		"inputs": [
			{"name": "user", "type": "address", "indexed": true},
			{"name": "token", "type": "address", "indexed": true},
			{"name": "amount", "type": "uint256", "indexed": false},
			{"name": "reason", "type": "string", "indexed": false}
		]
	}
]`

// WithdrawParams represents the parameters for a withdrawal.
type WithdrawParams struct {
	User      common.Address `json:"user"`
	Token     common.Address `json:"token"`
	Amount    *big.Int       `json:"amount"`
	To        common.Address `json:"to"`
	Nonce     *big.Int       `json:"nonce"`
	Signature []byte         `json:"signature"`
}

// DepositEvent represents the Deposit event from the Vault contract.
type DepositEvent struct {
	User   common.Address `json:"user"`
	Token  common.Address `json:"token"`
	Amount *big.Int       `json:"amount"`
	Raw    types.Log
}

// WithdrawEvent represents the Withdraw event from the Vault contract.
type WithdrawEvent struct {
	User   common.Address `json:"user"`
	Token  common.Address `json:"token"`
	Amount *big.Int       `json:"amount"`
	To     common.Address `json:"to"`
	Nonce  *big.Int       `json:"nonce"`
	Raw    types.Log
}

// WithdrawFailedEvent represents the WithdrawFailed event from the Vault contract.
type WithdrawFailedEvent struct {
	User   common.Address `json:"user"`
	Token  common.Address `json:"token"`
	Amount *big.Int       `json:"amount"`
	Reason string         `json:"reason"`
	Raw    types.Log
}

// VaultContract provides methods to interact with the Vault smart contract.
type VaultContract struct {
	address common.Address
	abi     abi.ABI
	caller  bind.ContractCaller
	backend bind.ContractBackend
}

// NewVaultContract creates a new Vault contract instance.
func NewVaultContract(address common.Address, backend bind.ContractBackend) (*VaultContract, error) {
	parsed, err := abi.JSON(strings.NewReader(VaultABI))
	if err != nil {
		return nil, err
	}

	return &VaultContract{
		address: address,
		abi:     parsed,
		backend: backend,
		caller:  backend,
	}, nil
}

// Address returns the contract address.
func (c *VaultContract) Address() common.Address {
	return c.address
}

// ABI returns the contract ABI.
func (c *VaultContract) ABI() abi.ABI {
	return c.abi
}

// PackWithdraw packs the withdraw function call data.
func (c *VaultContract) PackWithdraw(params *WithdrawParams) ([]byte, error) {
	if params == nil {
		return nil, ErrInvalidWithdrawalData
	}
	if params.Amount == nil || params.Amount.Sign() <= 0 {
		return nil, errors.New("invalid withdrawal amount")
	}
	if params.Nonce == nil {
		return nil, errors.New("invalid nonce")
	}

	return c.abi.Pack(
		"withdraw",
		params.User,
		params.Token,
		params.Amount,
		params.To,
		params.Nonce,
		params.Signature,
	)
}

// PackBatchWithdraw packs the batchWithdraw function call data.
func (c *VaultContract) PackBatchWithdraw(params []*WithdrawParams) ([]byte, error) {
	if len(params) == 0 {
		return nil, errors.New("empty withdrawal list")
	}
	return c.abi.Pack("batchWithdraw", params)
}

// Withdraw creates a transaction to execute a withdrawal.
func (c *VaultContract) Withdraw(opts *bind.TransactOpts, params *WithdrawParams) (*types.Transaction, error) {
	data, err := c.PackWithdraw(params)
	if err != nil {
		return nil, err
	}

	tx := types.NewTransaction(
		opts.Nonce.Uint64(),
		c.address,
		big.NewInt(0), // No value transfer
		opts.GasLimit,
		opts.GasPrice,
		data,
	)

	return tx, nil
}

// GetBalance queries the balance of a user for a specific token.
func (c *VaultContract) GetBalance(ctx context.Context, user, token common.Address) (*big.Int, error) {
	data, err := c.abi.Pack("getBalance", user, token)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &c.address,
		Data: data,
	}

	result, err := c.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	var balance *big.Int
	err = c.abi.UnpackIntoInterface(&balance, "getBalance", result)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

// GetNonce queries the current nonce for a user.
func (c *VaultContract) GetNonce(ctx context.Context, user common.Address) (*big.Int, error) {
	data, err := c.abi.Pack("getNonce", user)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &c.address,
		Data: data,
	}

	result, err := c.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	var nonce *big.Int
	err = c.abi.UnpackIntoInterface(&nonce, "getNonce", result)
	if err != nil {
		return nil, err
	}

	return nonce, nil
}

// IsWithdrawalProcessed checks if a withdrawal has been processed.
func (c *VaultContract) IsWithdrawalProcessed(ctx context.Context, user common.Address, nonce *big.Int) (bool, error) {
	data, err := c.abi.Pack("isWithdrawalProcessed", user, nonce)
	if err != nil {
		return false, err
	}

	msg := ethereum.CallMsg{
		To:   &c.address,
		Data: data,
	}

	result, err := c.caller.CallContract(ctx, msg, nil)
	if err != nil {
		return false, err
	}

	var processed bool
	err = c.abi.UnpackIntoInterface(&processed, "isWithdrawalProcessed", result)
	if err != nil {
		return false, err
	}

	return processed, nil
}

// EstimateGas estimates the gas required for a withdrawal.
func (c *VaultContract) EstimateGas(ctx context.Context, from common.Address, params *WithdrawParams) (uint64, error) {
	data, err := c.PackWithdraw(params)
	if err != nil {
		return 0, err
	}

	msg := ethereum.CallMsg{
		From: from,
		To:   &c.address,
		Data: data,
	}

	return c.backend.EstimateGas(ctx, msg)
}

// ParseDeposit parses a Deposit event from a log.
func (c *VaultContract) ParseDeposit(log types.Log) (*DepositEvent, error) {
	event := &DepositEvent{}
	event.Raw = log

	// Parse indexed fields from topics
	if len(log.Topics) < 3 {
		return nil, errors.New("not enough topics for Deposit event")
	}
	event.User = common.HexToAddress(log.Topics[1].Hex())
	event.Token = common.HexToAddress(log.Topics[2].Hex())

	// Parse non-indexed fields from data
	if len(log.Data) >= 32 {
		event.Amount = new(big.Int).SetBytes(log.Data[:32])
	}

	return event, nil
}

// ParseWithdraw parses a Withdraw event from a log.
func (c *VaultContract) ParseWithdraw(log types.Log) (*WithdrawEvent, error) {
	event := &WithdrawEvent{}
	event.Raw = log

	// Parse indexed fields from topics
	if len(log.Topics) < 3 {
		return nil, errors.New("not enough topics for Withdraw event")
	}
	event.User = common.HexToAddress(log.Topics[1].Hex())
	event.Token = common.HexToAddress(log.Topics[2].Hex())

	// Parse non-indexed fields from data
	if len(log.Data) >= 96 {
		event.Amount = new(big.Int).SetBytes(log.Data[:32])
		event.To = common.BytesToAddress(log.Data[32:64])
		event.Nonce = new(big.Int).SetBytes(log.Data[64:96])
	}

	return event, nil
}

// ParseWithdrawFailed parses a WithdrawFailed event from a log.
func (c *VaultContract) ParseWithdrawFailed(log types.Log) (*WithdrawFailedEvent, error) {
	event := &WithdrawFailedEvent{}
	event.Raw = log

	// Parse indexed fields from topics
	if len(log.Topics) < 3 {
		return nil, errors.New("not enough topics for WithdrawFailed event")
	}
	event.User = common.HexToAddress(log.Topics[1].Hex())
	event.Token = common.HexToAddress(log.Topics[2].Hex())

	// Parse non-indexed fields from data using ABI
	err := c.abi.UnpackIntoInterface(event, "WithdrawFailed", log.Data)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// DepositEventTopic returns the topic for Deposit events.
func (c *VaultContract) DepositEventTopic() common.Hash {
	return c.abi.Events["Deposit"].ID
}

// WithdrawEventTopic returns the topic for Withdraw events.
func (c *VaultContract) WithdrawEventTopic() common.Hash {
	return c.abi.Events["Withdraw"].ID
}

// WithdrawFailedEventTopic returns the topic for WithdrawFailed events.
func (c *VaultContract) WithdrawFailedEventTopic() common.Hash {
	return c.abi.Events["WithdrawFailed"].ID
}

// WatchDeposits watches for Deposit events.
func (c *VaultContract) WatchDeposits(
	ctx context.Context,
	sink chan<- *DepositEvent,
	user []common.Address,
	token []common.Address,
) (event.Subscription, error) {
	// Build filter query
	topics := [][]common.Hash{{c.DepositEventTopic()}}

	if len(user) > 0 {
		userTopics := make([]common.Hash, len(user))
		for i, addr := range user {
			userTopics[i] = common.BytesToHash(addr.Bytes())
		}
		topics = append(topics, userTopics)
	} else {
		topics = append(topics, nil)
	}

	if len(token) > 0 {
		tokenTopics := make([]common.Hash, len(token))
		for i, addr := range token {
			tokenTopics[i] = common.BytesToHash(addr.Bytes())
		}
		topics = append(topics, tokenTopics)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{c.address},
		Topics:    topics,
	}

	logs := make(chan types.Log)
	sub, err := c.backend.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case log := <-logs:
				event, err := c.ParseDeposit(log)
				if err == nil {
					sink <- event
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return sub, nil
}

// WatchWithdraws watches for Withdraw events.
func (c *VaultContract) WatchWithdraws(
	ctx context.Context,
	sink chan<- *WithdrawEvent,
	user []common.Address,
	token []common.Address,
) (event.Subscription, error) {
	// Build filter query
	topics := [][]common.Hash{{c.WithdrawEventTopic()}}

	if len(user) > 0 {
		userTopics := make([]common.Hash, len(user))
		for i, addr := range user {
			userTopics[i] = common.BytesToHash(addr.Bytes())
		}
		topics = append(topics, userTopics)
	} else {
		topics = append(topics, nil)
	}

	if len(token) > 0 {
		tokenTopics := make([]common.Hash, len(token))
		for i, addr := range token {
			tokenTopics[i] = common.BytesToHash(addr.Bytes())
		}
		topics = append(topics, tokenTopics)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{c.address},
		Topics:    topics,
	}

	logs := make(chan types.Log)
	sub, err := c.backend.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case log := <-logs:
				event, err := c.ParseWithdraw(log)
				if err == nil {
					sink <- event
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return sub, nil
}

// NativeToken returns the zero address representing native ETH.
func NativeToken() common.Address {
	return common.Address{}
}

// IsNativeToken checks if an address represents the native token.
func IsNativeToken(token common.Address) bool {
	return token == NativeToken()
}
