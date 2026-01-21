// Package contract provides smart contract ABI bindings for the Eidos Exchange.
// These bindings are generated based on the contract ABIs and provide type-safe
// methods for interacting with the Exchange and Vault smart contracts.
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

// Exchange contract errors
var (
	ErrExchangeContractNotDeployed = errors.New("exchange contract not deployed")
	ErrInvalidSettlementData       = errors.New("invalid settlement data")
	ErrEmptyTradeIDs               = errors.New("empty trade IDs")
)

// ExchangeABI is the ABI of the Exchange smart contract.
// This matches the Solidity contract interface:
//
//	function batchSettle(SettleTrade[] calldata trades) external;
//	function getTradeStatus(bytes32 tradeId) external view returns (TradeStatus);
//	event SettlementExecuted(bytes32 indexed batchId, uint256 tradeCount, uint256 timestamp);
const ExchangeABI = `[
	{
		"type": "function",
		"name": "batchSettle",
		"inputs": [
			{
				"name": "trades",
				"type": "tuple[]",
				"components": [
					{"name": "tradeId", "type": "bytes32"},
					{"name": "makerWallet", "type": "address"},
					{"name": "takerWallet", "type": "address"},
					{"name": "baseToken", "type": "address"},
					{"name": "quoteToken", "type": "address"},
					{"name": "baseAmount", "type": "uint256"},
					{"name": "quoteAmount", "type": "uint256"},
					{"name": "makerFee", "type": "uint256"},
					{"name": "takerFee", "type": "uint256"},
					{"name": "makerSide", "type": "uint8"}
				]
			}
		],
		"outputs": [
			{"name": "success", "type": "bool"}
		],
		"stateMutability": "nonpayable"
	},
	{
		"type": "function",
		"name": "getTradeStatus",
		"inputs": [
			{"name": "tradeId", "type": "bytes32"}
		],
		"outputs": [
			{
				"name": "status",
				"type": "tuple",
				"components": [
					{"name": "settled", "type": "bool"},
					{"name": "settledAt", "type": "uint256"},
					{"name": "batchId", "type": "bytes32"}
				]
			}
		],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "getBatchStatus",
		"inputs": [
			{"name": "batchId", "type": "bytes32"}
		],
		"outputs": [
			{
				"name": "status",
				"type": "tuple",
				"components": [
					{"name": "executed", "type": "bool"},
					{"name": "tradeCount", "type": "uint256"},
					{"name": "executedAt", "type": "uint256"}
				]
			}
		],
		"stateMutability": "view"
	},
	{
		"type": "event",
		"name": "SettlementExecuted",
		"inputs": [
			{"name": "batchId", "type": "bytes32", "indexed": true},
			{"name": "tradeCount", "type": "uint256", "indexed": false},
			{"name": "timestamp", "type": "uint256", "indexed": false}
		]
	},
	{
		"type": "event",
		"name": "TradeFailed",
		"inputs": [
			{"name": "tradeId", "type": "bytes32", "indexed": true},
			{"name": "reason", "type": "string", "indexed": false}
		]
	}
]`

// SettleTrade represents a trade to be settled on-chain.
type SettleTrade struct {
	TradeID     [32]byte       `json:"tradeId"`
	MakerWallet common.Address `json:"makerWallet"`
	TakerWallet common.Address `json:"takerWallet"`
	BaseToken   common.Address `json:"baseToken"`
	QuoteToken  common.Address `json:"quoteToken"`
	BaseAmount  *big.Int       `json:"baseAmount"`
	QuoteAmount *big.Int       `json:"quoteAmount"`
	MakerFee    *big.Int       `json:"makerFee"`
	TakerFee    *big.Int       `json:"takerFee"`
	MakerSide   uint8          `json:"makerSide"` // 0 = buy, 1 = sell
}

// TradeStatus represents the on-chain status of a trade.
type TradeStatus struct {
	Settled   bool     `json:"settled"`
	SettledAt *big.Int `json:"settledAt"`
	BatchID   [32]byte `json:"batchId"`
}

// BatchStatus represents the on-chain status of a settlement batch.
type BatchStatus struct {
	Executed   bool     `json:"executed"`
	TradeCount *big.Int `json:"tradeCount"`
	ExecutedAt *big.Int `json:"executedAt"`
}

// SettlementExecutedEvent represents the SettlementExecuted event from the contract.
type SettlementExecutedEvent struct {
	BatchID    [32]byte `json:"batchId"`
	TradeCount *big.Int `json:"tradeCount"`
	Timestamp  *big.Int `json:"timestamp"`
	Raw        types.Log
}

// TradeFailedEvent represents the TradeFailed event from the contract.
type TradeFailedEvent struct {
	TradeID [32]byte `json:"tradeId"`
	Reason  string   `json:"reason"`
	Raw     types.Log
}

// ExchangeContract provides methods to interact with the Exchange smart contract.
type ExchangeContract struct {
	address common.Address
	abi     abi.ABI
	caller  bind.ContractCaller
	backend bind.ContractBackend
}

// NewExchangeContract creates a new Exchange contract instance.
func NewExchangeContract(address common.Address, backend bind.ContractBackend) (*ExchangeContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ExchangeABI))
	if err != nil {
		return nil, err
	}

	return &ExchangeContract{
		address: address,
		abi:     parsed,
		backend: backend,
		caller:  backend,
	}, nil
}

// Address returns the contract address.
func (c *ExchangeContract) Address() common.Address {
	return c.address
}

// ABI returns the contract ABI.
func (c *ExchangeContract) ABI() abi.ABI {
	return c.abi
}

// PackBatchSettle packs the batchSettle function call data.
func (c *ExchangeContract) PackBatchSettle(trades []SettleTrade) ([]byte, error) {
	if len(trades) == 0 {
		return nil, ErrEmptyTradeIDs
	}
	return c.abi.Pack("batchSettle", trades)
}

// BatchSettle creates a transaction to settle a batch of trades.
func (c *ExchangeContract) BatchSettle(opts *bind.TransactOpts, trades []SettleTrade) (*types.Transaction, error) {
	if len(trades) == 0 {
		return nil, ErrEmptyTradeIDs
	}

	data, err := c.PackBatchSettle(trades)
	if err != nil {
		return nil, err
	}

	// Create the transaction
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

// GetTradeStatus queries the on-chain status of a trade.
func (c *ExchangeContract) GetTradeStatus(ctx context.Context, tradeID [32]byte) (*TradeStatus, error) {
	data, err := c.abi.Pack("getTradeStatus", tradeID)
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

	var status TradeStatus
	err = c.abi.UnpackIntoInterface(&status, "getTradeStatus", result)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// GetBatchStatus queries the on-chain status of a settlement batch.
func (c *ExchangeContract) GetBatchStatus(ctx context.Context, batchID [32]byte) (*BatchStatus, error) {
	data, err := c.abi.Pack("getBatchStatus", batchID)
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

	var status BatchStatus
	err = c.abi.UnpackIntoInterface(&status, "getBatchStatus", result)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// EstimateGas estimates the gas required for batchSettle.
func (c *ExchangeContract) EstimateGas(ctx context.Context, from common.Address, trades []SettleTrade) (uint64, error) {
	data, err := c.PackBatchSettle(trades)
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

// ParseSettlementExecuted parses a SettlementExecuted event from a log.
func (c *ExchangeContract) ParseSettlementExecuted(log types.Log) (*SettlementExecutedEvent, error) {
	event := &SettlementExecutedEvent{}
	event.Raw = log

	// Parse indexed fields from topics
	if len(log.Topics) < 2 {
		return nil, errors.New("not enough topics for SettlementExecuted event")
	}
	copy(event.BatchID[:], log.Topics[1].Bytes())

	// Parse non-indexed fields from data
	err := c.abi.UnpackIntoInterface(event, "SettlementExecuted", log.Data)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// ParseTradeFailed parses a TradeFailed event from a log.
func (c *ExchangeContract) ParseTradeFailed(log types.Log) (*TradeFailedEvent, error) {
	event := &TradeFailedEvent{}
	event.Raw = log

	// Parse indexed fields from topics
	if len(log.Topics) < 2 {
		return nil, errors.New("not enough topics for TradeFailed event")
	}
	copy(event.TradeID[:], log.Topics[1].Bytes())

	// Parse non-indexed fields from data
	err := c.abi.UnpackIntoInterface(event, "TradeFailed", log.Data)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// SettlementExecutedEventTopic returns the topic for SettlementExecuted events.
func (c *ExchangeContract) SettlementExecutedEventTopic() common.Hash {
	return c.abi.Events["SettlementExecuted"].ID
}

// TradeFailedEventTopic returns the topic for TradeFailed events.
func (c *ExchangeContract) TradeFailedEventTopic() common.Hash {
	return c.abi.Events["TradeFailed"].ID
}

// WatchSettlementExecuted watches for SettlementExecuted events.
func (c *ExchangeContract) WatchSettlementExecuted(
	ctx context.Context,
	sink chan<- *SettlementExecutedEvent,
	batchID [][32]byte,
) (event.Subscription, error) {
	// Build filter query
	topics := [][]common.Hash{{c.SettlementExecutedEventTopic()}}

	if len(batchID) > 0 {
		batchIDTopics := make([]common.Hash, len(batchID))
		for i, id := range batchID {
			batchIDTopics[i] = common.BytesToHash(id[:])
		}
		topics = append(topics, batchIDTopics)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{c.address},
		Topics:    topics,
	}

	// Subscribe to filter logs
	logs := make(chan types.Log)
	sub, err := c.backend.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case log := <-logs:
				event, err := c.ParseSettlementExecuted(log)
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

// StringToBytes32 converts a string to a [32]byte array.
func StringToBytes32(s string) [32]byte {
	var result [32]byte
	copy(result[:], []byte(s))
	return result
}

// Bytes32ToString converts a [32]byte array to a string, trimming null bytes.
func Bytes32ToString(b [32]byte) string {
	// Find the null terminator
	n := 0
	for n < 32 && b[n] != 0 {
		n++
	}
	return string(b[:n])
}
