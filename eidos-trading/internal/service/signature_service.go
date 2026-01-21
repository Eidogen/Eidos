// Package service provides signature verification using EIP-712
package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/crypto"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// Signature verification errors
var (
	ErrSignatureInvalid     = errors.New("invalid signature")
	ErrSignatureMismatch    = errors.New("signature does not match wallet")
	ErrSignatureExpired     = errors.New("signature has expired")
	ErrMockModeNoValidation = errors.New("signature validation skipped in mock mode")
)

// SignatureService provides EIP-712 signature verification
type SignatureService interface {
	// VerifyOrderSignature verifies an order signature
	VerifyOrderSignature(ctx context.Context, order *CreateOrderRequest) error

	// VerifyCancelSignature verifies a cancel order signature
	VerifyCancelSignature(ctx context.Context, wallet, orderID string, nonce uint64, signature []byte) error

	// VerifyWithdrawalSignature verifies a withdrawal signature
	VerifyWithdrawalSignature(ctx context.Context, req *CreateWithdrawalRequest) error

	// IsMockMode returns true if running in mock mode (no signature validation)
	IsMockMode() bool
}

// signatureService implements SignatureService
type signatureService struct {
	domain   crypto.EIP712Domain
	mockMode bool
}

// NewSignatureService creates a new signature service
func NewSignatureService(cfg *config.EIP712Config) SignatureService {
	domain := crypto.EIP712Domain{
		Name:              cfg.Domain.Name,
		Version:           cfg.Domain.Version,
		ChainID:           cfg.Domain.ChainID,
		VerifyingContract: cfg.Domain.VerifyingContract,
	}

	// Check if running in mock mode (zero contract address)
	mockMode := crypto.IsMockMode(domain)

	return &signatureService{
		domain:   domain,
		mockMode: mockMode,
	}
}

// VerifyOrderSignature verifies an order signature
func (s *signatureService) VerifyOrderSignature(ctx context.Context, req *CreateOrderRequest) error {
	// Skip validation in mock mode
	if s.mockMode {
		return nil
	}

	// Convert decimal to big.Int (multiply by 10^18 for precision)
	priceWei := decimalToBigInt(req.Price, 18)
	amountWei := decimalToBigInt(req.Amount, 18)

	// Map order side
	var side uint8 = 0 // buy
	if req.Side == model.OrderSideSell {
		side = 1 // sell
	}

	// Map order type
	var orderType uint8 = 0 // limit
	if req.Type == model.OrderTypeMarket {
		orderType = 1 // market
	}

	orderData := crypto.OrderData{
		Wallet:    req.Wallet,
		Market:    req.Market,
		Side:      side,
		OrderType: orderType,
		Price:     priceWei,
		Amount:    amountWei,
		Nonce:     req.Nonce,
		Expiry:    req.ExpireAt / 1000, // Convert milliseconds to seconds
	}

	valid, err := crypto.VerifyOrderSignature(s.domain, orderData, req.Signature)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}

	if !valid {
		return ErrSignatureMismatch
	}

	return nil
}

// VerifyCancelSignature verifies a cancel order signature
func (s *signatureService) VerifyCancelSignature(ctx context.Context, wallet, orderID string, nonce uint64, signature []byte) error {
	// Skip validation in mock mode
	if s.mockMode {
		return nil
	}

	cancelData := crypto.CancelData{
		Wallet:  wallet,
		OrderID: orderID,
		Nonce:   nonce,
	}

	valid, err := crypto.VerifyCancelSignature(s.domain, cancelData, signature)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}

	if !valid {
		return ErrSignatureMismatch
	}

	return nil
}

// VerifyWithdrawalSignature verifies a withdrawal signature
func (s *signatureService) VerifyWithdrawalSignature(ctx context.Context, req *CreateWithdrawalRequest) error {
	// Skip validation in mock mode
	if s.mockMode {
		return nil
	}

	// Convert decimal to big.Int
	amountWei := decimalToBigInt(req.Amount, 18)

	withdrawData := crypto.WithdrawData{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    amountWei,
		ToAddress: req.ToAddress,
		Nonce:     req.Nonce,
	}

	valid, err := crypto.VerifyWithdrawalSignature(s.domain, withdrawData, req.Signature)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}

	if !valid {
		return ErrSignatureMismatch
	}

	return nil
}

// IsMockMode returns true if running in mock mode
func (s *signatureService) IsMockMode() bool {
	return s.mockMode
}

// decimalToBigInt converts a decimal to big.Int with specified decimals
func decimalToBigInt(d decimal.Decimal, decimals int32) *big.Int {
	// Multiply by 10^decimals
	multiplier := decimal.NewFromInt(10).Pow(decimal.NewFromInt32(decimals))
	result := d.Mul(multiplier)

	// Convert to big.Int
	bigInt := new(big.Int)
	bigInt.SetString(result.Truncate(0).String(), 10)
	return bigInt
}
