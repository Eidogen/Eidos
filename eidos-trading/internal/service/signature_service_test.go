package service

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	eip712 "github.com/eidos-exchange/eidos/eidos-common/pkg/crypto"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

func TestSignatureService_MockMode(t *testing.T) {
	// Mock mode configuration (zero address)
	cfg := &config.EIP712Config{
		Domain: config.EIP712Domain{
			Name:              "EidosExchange",
			Version:           "1",
			ChainID:           31337,
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
	}

	svc := NewSignatureService(cfg)

	assert.True(t, svc.IsMockMode())

	// In mock mode, verification should always pass
	ctx := context.Background()
	err := svc.VerifyOrderSignature(ctx, &CreateOrderRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		ExpireAt:  9999999999999,
		Signature: []byte("invalid-signature"),
	})

	assert.NoError(t, err)
}

func TestSignatureService_VerifyOrderSignature(t *testing.T) {
	// Generate a test private key
	privateKey, err := crypto.GenerateKey()
	assert.NoError(t, err)

	wallet := eip712.AddressFromPrivateKey(privateKey)

	// Non-mock mode configuration
	cfg := &config.EIP712Config{
		Domain: config.EIP712Domain{
			Name:              "EidosExchange",
			Version:           "1",
			ChainID:           31337,
			VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		},
	}

	svc := NewSignatureService(cfg)

	assert.False(t, svc.IsMockMode())

	// Create order data
	price := new(big.Int)
	price.SetString("2000000000000000000000", 10) // 2000 * 10^18
	amount := big.NewInt(1000000000000000000)     // 1 * 10^18

	orderData := eip712.OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0, // buy
		OrderType: 0, // limit
		Price:     price,
		Amount:    amount,
		Nonce:     1,
		Expiry:    9999999999,
	}

	// Sign the order
	domain := eip712.EIP712Domain{
		Name:              cfg.Domain.Name,
		Version:           cfg.Domain.Version,
		ChainID:           cfg.Domain.ChainID,
		VerifyingContract: cfg.Domain.VerifyingContract,
	}

	signature, err := eip712.SignOrder(privateKey, domain, orderData)
	assert.NoError(t, err)

	// Verify the signature
	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		ExpireAt:  9999999999000, // milliseconds
		Signature: signature,
	}

	err = svc.VerifyOrderSignature(ctx, req)
	assert.NoError(t, err)
}

func TestSignatureService_VerifyOrderSignature_InvalidSignature(t *testing.T) {
	// Non-mock mode configuration
	cfg := &config.EIP712Config{
		Domain: config.EIP712Domain{
			Name:              "EidosExchange",
			Version:           "1",
			ChainID:           31337,
			VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		},
	}

	svc := NewSignatureService(cfg)

	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		ExpireAt:  9999999999000,
		Signature: make([]byte, 65), // Invalid signature (all zeros)
	}

	err := svc.VerifyOrderSignature(ctx, req)
	assert.Error(t, err)
}

func TestSignatureService_VerifyCancelSignature(t *testing.T) {
	// Generate a test private key
	privateKey, err := crypto.GenerateKey()
	assert.NoError(t, err)

	wallet := eip712.AddressFromPrivateKey(privateKey)

	// Non-mock mode configuration
	cfg := &config.EIP712Config{
		Domain: config.EIP712Domain{
			Name:              "EidosExchange",
			Version:           "1",
			ChainID:           31337,
			VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		},
	}

	svc := NewSignatureService(cfg)

	// Create cancel data
	cancelData := eip712.CancelData{
		Wallet:  wallet,
		OrderID: "O123456789",
		Nonce:   1,
	}

	// Sign the cancel
	domain := eip712.EIP712Domain{
		Name:              cfg.Domain.Name,
		Version:           cfg.Domain.Version,
		ChainID:           cfg.Domain.ChainID,
		VerifyingContract: cfg.Domain.VerifyingContract,
	}

	signature, err := eip712.SignCancel(privateKey, domain, cancelData)
	assert.NoError(t, err)

	// Verify the signature
	ctx := context.Background()
	err = svc.VerifyCancelSignature(ctx, wallet, "O123456789", 1, signature)
	assert.NoError(t, err)
}

func TestSignatureService_VerifyWithdrawalSignature(t *testing.T) {
	// Generate a test private key
	privateKey, err := crypto.GenerateKey()
	assert.NoError(t, err)

	wallet := eip712.AddressFromPrivateKey(privateKey)

	// Non-mock mode configuration
	cfg := &config.EIP712Config{
		Domain: config.EIP712Domain{
			Name:              "EidosExchange",
			Version:           "1",
			ChainID:           31337,
			VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		},
	}

	svc := NewSignatureService(cfg)

	// Create withdrawal data
	amount := new(big.Int)
	amount.SetString("100000000000000000000", 10) // 100 * 10^18

	withdrawData := eip712.WithdrawData{
		Wallet:    wallet,
		Token:     "USDT",
		Amount:    amount,
		ToAddress: wallet,
		Nonce:     1,
	}

	// Sign the withdrawal
	domain := eip712.EIP712Domain{
		Name:              cfg.Domain.Name,
		Version:           cfg.Domain.Version,
		ChainID:           cfg.Domain.ChainID,
		VerifyingContract: cfg.Domain.VerifyingContract,
	}

	signature, err := eip712.SignWithdraw(privateKey, domain, withdrawData)
	assert.NoError(t, err)

	// Verify the signature
	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: wallet,
		Nonce:     1,
		Signature: signature,
	}

	err = svc.VerifyWithdrawalSignature(ctx, req)
	assert.NoError(t, err)
}

func TestDecimalToBigInt(t *testing.T) {
	testCases := []struct {
		name     string
		input    decimal.Decimal
		decimals int32
		expected string
	}{
		{
			name:     "simple integer",
			input:    decimal.NewFromFloat(100),
			decimals: 18,
			expected: "100000000000000000000",
		},
		{
			name:     "decimal value",
			input:    decimal.NewFromFloat(1.5),
			decimals: 18,
			expected: "1500000000000000000",
		},
		{
			name:     "small decimals",
			input:    decimal.NewFromFloat(100),
			decimals: 6,
			expected: "100000000",
		},
		{
			name:     "zero",
			input:    decimal.Zero,
			decimals: 18,
			expected: "0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := decimalToBigInt(tc.input, tc.decimals)
			assert.Equal(t, tc.expected, result.String())
		})
	}
}

// Helper to generate test keys
func generateTestKey() (*ecdsa.PrivateKey, string) {
	privateKey, _ := crypto.GenerateKey()
	address := eip712.AddressFromPrivateKey(privateKey)
	return privateKey, address
}
