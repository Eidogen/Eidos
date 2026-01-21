// Package crypto provides comprehensive tests for EIP-712 typed data signing and verification.
package crypto

import (
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test Fixtures
// =============================================================================

// testPrivateKey is a well-known test private key (DO NOT use in production).
// This corresponds to one of the default Hardhat/Anvil test accounts.
const testPrivateKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

// testDomain provides a consistent domain configuration for tests.
var testDomain = EIP712Domain{
	Name:              "EidosExchange",
	Version:           "1",
	ChainID:           31337,
	VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
}

// getTestPrivateKey returns the test private key.
func getTestPrivateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	privateKey, err := PrivateKeyFromHex(testPrivateKeyHex)
	require.NoError(t, err, "failed to parse test private key")
	return privateKey
}

// getTestWalletAddress returns the address derived from the test private key.
func getTestWalletAddress(t *testing.T) string {
	t.Helper()
	privateKey := getTestPrivateKey(t)
	return AddressFromPrivateKey(privateKey)
}

// =============================================================================
// Keccak256 Tests
// =============================================================================

func TestKeccak256(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
		},
		{
			name:     "hello world",
			input:    []byte("hello"),
			expected: "1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8",
		},
		{
			name:     "EIP712Domain type string",
			input:    []byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
			expected: "8b73c3c69bb8fe3d512ecc4cf759cc79239f7b179b0ffacaa9a75d522b39400f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Keccak256(tt.input)
			assert.Equal(t, tt.expected, hex.EncodeToString(result))
		})
	}
}

func TestKeccak256Hash(t *testing.T) {
	result := Keccak256Hash([]byte("hello"))
	assert.True(t, strings.HasPrefix(result, "0x"), "result should have 0x prefix")
	assert.Equal(t, "0x1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8", result)
}

// =============================================================================
// Domain Validation Tests
// =============================================================================

func TestEIP712Domain_Validate(t *testing.T) {
	tests := []struct {
		name    string
		domain  EIP712Domain
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid domain",
			domain:  testDomain,
			wantErr: false,
		},
		{
			name: "empty name",
			domain: EIP712Domain{
				Name:              "",
				Version:           "1",
				ChainID:           1,
				VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
			},
			wantErr: true,
			errMsg:  "domain name cannot be empty",
		},
		{
			name: "empty version",
			domain: EIP712Domain{
				Name:              "Test",
				Version:           "",
				ChainID:           1,
				VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
			},
			wantErr: true,
			errMsg:  "domain version cannot be empty",
		},
		{
			name: "zero chain ID",
			domain: EIP712Domain{
				Name:              "Test",
				Version:           "1",
				ChainID:           0,
				VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
			},
			wantErr: true,
			errMsg:  "chain ID must be positive",
		},
		{
			name: "negative chain ID",
			domain: EIP712Domain{
				Name:              "Test",
				Version:           "1",
				ChainID:           -1,
				VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
			},
			wantErr: true,
			errMsg:  "chain ID must be positive",
		},
		{
			name: "invalid contract address - too short",
			domain: EIP712Domain{
				Name:              "Test",
				Version:           "1",
				ChainID:           1,
				VerifyingContract: "0x5FbDB2315678afecb367f032d93F642f64180",
			},
			wantErr: true,
		},
		{
			name: "invalid contract address - no prefix",
			domain: EIP712Domain{
				Name:              "Test",
				Version:           "1",
				ChainID:           1,
				VerifyingContract: "5FbDB2315678afecb367f032d93F642f64180aa3",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.domain.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsMockMode(t *testing.T) {
	tests := []struct {
		name     string
		domain   EIP712Domain
		expected bool
	}{
		{
			name:     "mock mode - zero address",
			domain:   DefaultDomain,
			expected: true,
		},
		{
			name:     "production mode - real address",
			domain:   testDomain,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsMockMode(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Data Structure Validation Tests
// =============================================================================

func TestOrderData_Validate(t *testing.T) {
	validOrder := OrderData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    time.Now().Add(24 * time.Hour).Unix(),
	}

	tests := []struct {
		name    string
		modify  func(*OrderData)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid order",
			modify:  func(o *OrderData) {},
			wantErr: false,
		},
		{
			name:    "invalid wallet",
			modify:  func(o *OrderData) { o.Wallet = "invalid" },
			wantErr: true,
		},
		{
			name:    "empty market",
			modify:  func(o *OrderData) { o.Market = "" },
			wantErr: true,
			errMsg:  "market cannot be empty",
		},
		{
			name:    "invalid side",
			modify:  func(o *OrderData) { o.Side = 2 },
			wantErr: true,
			errMsg:  "invalid order side",
		},
		{
			name:    "invalid order type",
			modify:  func(o *OrderData) { o.OrderType = 2 },
			wantErr: true,
			errMsg:  "invalid order type",
		},
		{
			name:    "nil price",
			modify:  func(o *OrderData) { o.Price = nil },
			wantErr: true,
			errMsg:  "price cannot be nil",
		},
		{
			name:    "nil amount",
			modify:  func(o *OrderData) { o.Amount = nil },
			wantErr: true,
			errMsg:  "amount cannot be nil",
		},
		{
			name:    "zero amount",
			modify:  func(o *OrderData) { o.Amount = big.NewInt(0) },
			wantErr: true,
			errMsg:  "amount must be positive",
		},
		{
			name:    "negative amount",
			modify:  func(o *OrderData) { o.Amount = big.NewInt(-1) },
			wantErr: true,
			errMsg:  "amount must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := validOrder
			order.Price = new(big.Int).Set(validOrder.Price)
			order.Amount = new(big.Int).Set(validOrder.Amount)
			tt.modify(&order)

			err := order.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWithdrawData_Validate(t *testing.T) {
	validWithdraw := WithdrawData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Token:     "ETH",
		Amount:    big.NewInt(1000000000000000000),
		ToAddress: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
		Nonce:     1,
	}

	tests := []struct {
		name    string
		modify  func(*WithdrawData)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid withdrawal",
			modify:  func(w *WithdrawData) {},
			wantErr: false,
		},
		{
			name:    "invalid wallet",
			modify:  func(w *WithdrawData) { w.Wallet = "invalid" },
			wantErr: true,
		},
		{
			name:    "empty token",
			modify:  func(w *WithdrawData) { w.Token = "" },
			wantErr: true,
			errMsg:  "token cannot be empty",
		},
		{
			name:    "nil amount",
			modify:  func(w *WithdrawData) { w.Amount = nil },
			wantErr: true,
			errMsg:  "amount cannot be nil",
		},
		{
			name:    "zero amount",
			modify:  func(w *WithdrawData) { w.Amount = big.NewInt(0) },
			wantErr: true,
			errMsg:  "withdrawal amount must be positive",
		},
		{
			name:    "empty destination",
			modify:  func(w *WithdrawData) { w.ToAddress = "" },
			wantErr: true,
			errMsg:  "destination address cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withdraw := validWithdraw
			withdraw.Amount = new(big.Int).Set(validWithdraw.Amount)
			tt.modify(&withdraw)

			err := withdraw.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoginData_Validate(t *testing.T) {
	validLogin := LoginData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Message:   "Sign in to EidosExchange",
		Timestamp: time.Now().Unix(),
		Nonce:     1,
	}

	tests := []struct {
		name    string
		modify  func(*LoginData)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid login",
			modify:  func(l *LoginData) {},
			wantErr: false,
		},
		{
			name:    "invalid wallet",
			modify:  func(l *LoginData) { l.Wallet = "invalid" },
			wantErr: true,
		},
		{
			name:    "empty message",
			modify:  func(l *LoginData) { l.Message = "" },
			wantErr: true,
			errMsg:  "message cannot be empty",
		},
		{
			name:    "zero timestamp",
			modify:  func(l *LoginData) { l.Timestamp = 0 },
			wantErr: true,
			errMsg:  "timestamp must be positive",
		},
		{
			name:    "negative timestamp",
			modify:  func(l *LoginData) { l.Timestamp = -1 },
			wantErr: true,
			errMsg:  "timestamp must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			login := validLogin
			tt.modify(&login)

			err := login.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCancelData_Validate(t *testing.T) {
	validCancel := CancelData{
		Wallet:  "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		OrderID: "order-12345",
		Nonce:   1,
	}

	tests := []struct {
		name    string
		modify  func(*CancelData)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid cancel",
			modify:  func(c *CancelData) {},
			wantErr: false,
		},
		{
			name:    "invalid wallet",
			modify:  func(c *CancelData) { c.Wallet = "invalid" },
			wantErr: true,
		},
		{
			name:    "empty order ID",
			modify:  func(c *CancelData) { c.OrderID = "" },
			wantErr: true,
			errMsg:  "order ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cancel := validCancel
			tt.modify(&cancel)

			err := cancel.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Hash Function Tests
// =============================================================================

func TestHashTypedDataDomain(t *testing.T) {
	// Test that domain hashing is deterministic
	hash1 := HashTypedDataDomain(testDomain)
	hash2 := HashTypedDataDomain(testDomain)
	assert.Equal(t, hash1, hash2, "domain hash should be deterministic")

	// Test that different domains produce different hashes
	domain2 := testDomain
	domain2.ChainID = 1
	hash3 := HashTypedDataDomain(domain2)
	assert.NotEqual(t, hash1, hash3, "different chain IDs should produce different hashes")

	// Test that hash is 32 bytes
	assert.Len(t, hash1, 32, "domain hash should be 32 bytes")
}

func TestHashOrder(t *testing.T) {
	order := OrderData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    1700000000,
	}

	// Test determinism
	hash1 := HashOrder(order)
	hash2 := HashOrder(order)
	assert.Equal(t, hash1, hash2, "order hash should be deterministic")

	// Test that different orders produce different hashes
	order2 := order
	order2.Amount = big.NewInt(2000000000000000000)
	hash3 := HashOrder(order2)
	assert.NotEqual(t, hash1, hash3, "different amounts should produce different hashes")

	// Test hash length
	assert.Len(t, hash1, 32, "order hash should be 32 bytes")
}

func TestHashWithdraw(t *testing.T) {
	withdraw := WithdrawData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Token:     "ETH",
		Amount:    big.NewInt(1000000000000000000),
		ToAddress: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
		Nonce:     1,
	}

	hash1 := HashWithdraw(withdraw)
	hash2 := HashWithdraw(withdraw)
	assert.Equal(t, hash1, hash2, "withdrawal hash should be deterministic")
	assert.Len(t, hash1, 32, "withdrawal hash should be 32 bytes")
}

func TestHashLogin(t *testing.T) {
	login := LoginData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Message:   "Sign in to EidosExchange",
		Timestamp: 1700000000,
		Nonce:     1,
	}

	hash1 := HashLogin(login)
	hash2 := HashLogin(login)
	assert.Equal(t, hash1, hash2, "login hash should be deterministic")
	assert.Len(t, hash1, 32, "login hash should be 32 bytes")
}

func TestHashCancel(t *testing.T) {
	cancel := CancelData{
		Wallet:  "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		OrderID: "order-12345",
		Nonce:   1,
	}

	hash1 := HashCancel(cancel)
	hash2 := HashCancel(cancel)
	assert.Equal(t, hash1, hash2, "cancel hash should be deterministic")
	assert.Len(t, hash1, 32, "cancel hash should be 32 bytes")
}

func TestHashTypedDataV4(t *testing.T) {
	structHash := Keccak256([]byte("test struct"))
	hash1 := HashTypedDataV4(testDomain, structHash)
	hash2 := HashTypedDataV4(testDomain, structHash)

	assert.Equal(t, hash1, hash2, "typed data hash should be deterministic")
	assert.Len(t, hash1, 32, "typed data hash should be 32 bytes")

	// Different struct hash should produce different result
	structHash2 := Keccak256([]byte("different struct"))
	hash3 := HashTypedDataV4(testDomain, structHash2)
	assert.NotEqual(t, hash1, hash3, "different struct hashes should produce different results")
}

// =============================================================================
// Signature Tests
// =============================================================================

func TestRecoverAddress(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	expectedAddress := strings.ToLower(AddressFromPrivateKey(privateKey))

	// Create a hash
	hash := Keccak256([]byte("test message"))

	// Sign the hash
	sig, err := crypto.Sign(hash, privateKey)
	require.NoError(t, err)
	sig[64] += 27 // Convert to Ethereum format

	// Recover the address
	recovered, err := RecoverAddress(hash, sig)
	require.NoError(t, err)
	assert.Equal(t, expectedAddress, recovered)
}

func TestRecoverAddress_InvalidSignatureLength(t *testing.T) {
	hash := Keccak256([]byte("test"))

	tests := []struct {
		name    string
		sigLen  int
		wantErr bool
	}{
		{"too short", 64, true},
		{"too long", 66, true},
		{"correct length", 65, false},
	}

	privateKey := getTestPrivateKey(t)
	validSig, _ := crypto.Sign(hash, privateKey)
	validSig[64] += 27

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sig []byte
			if tt.sigLen == 65 {
				sig = validSig
			} else {
				sig = make([]byte, tt.sigLen)
			}

			_, err := RecoverAddress(hash, sig)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidSignatureLength)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRecoverAddress_InvalidHashLength(t *testing.T) {
	sig := make([]byte, 65)

	_, err := RecoverAddress([]byte("short"), sig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hash length")
}

func TestVerifySignature(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)
	hash := Keccak256([]byte("test message"))

	sig, err := crypto.Sign(hash, privateKey)
	require.NoError(t, err)
	sig[64] += 27

	t.Run("valid signature", func(t *testing.T) {
		valid, err := VerifySignature(wallet, hash, sig)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("wrong wallet address", func(t *testing.T) {
		wrongWallet := "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
		valid, err := VerifySignature(wrongWallet, hash, sig)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrAddressMismatch)
		assert.False(t, valid)
	})

	t.Run("invalid wallet format", func(t *testing.T) {
		valid, err := VerifySignature("invalid", hash, sig)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidAddress)
		assert.False(t, valid)
	})
}

// =============================================================================
// Order Signature Tests
// =============================================================================

func TestSignAndVerifyOrder(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    time.Now().Add(24 * time.Hour).Unix(),
	}

	// Sign the order
	sig, err := SignOrder(privateKey, testDomain, order)
	require.NoError(t, err)
	assert.Len(t, sig, 65, "signature should be 65 bytes")

	// Verify the signature
	valid, err := VerifyOrderSignature(testDomain, order, sig)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyOrderSignature_ExpiredOrder(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    time.Now().Add(-1 * time.Hour).Unix(), // Expired
	}

	sig, err := SignOrder(privateKey, testDomain, order)
	require.NoError(t, err)

	valid, err := VerifyOrderSignature(testDomain, order, sig)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureExpired)
	assert.False(t, valid)
}

func TestVerifyOrderSignature_ZeroExpiry(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	// Order with zero expiry (no expiration)
	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    0,
	}

	sig, err := SignOrder(privateKey, testDomain, order)
	require.NoError(t, err)

	valid, err := VerifyOrderSignature(testDomain, order, sig)
	require.NoError(t, err)
	assert.True(t, valid, "order with zero expiry should not expire")
}

func TestVerifyOrderSignature_WrongWallet(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    time.Now().Add(24 * time.Hour).Unix(),
	}

	sig, err := SignOrder(privateKey, testDomain, order)
	require.NoError(t, err)

	// Try to verify with a different wallet address
	order.Wallet = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
	valid, err := VerifyOrderSignature(testDomain, order, sig)
	assert.Error(t, err)
	assert.False(t, valid)
}

// =============================================================================
// Withdrawal Signature Tests
// =============================================================================

func TestSignAndVerifyWithdrawal(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	withdraw := WithdrawData{
		Wallet:    wallet,
		Token:     "ETH",
		Amount:    big.NewInt(1000000000000000000),
		ToAddress: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
		Nonce:     1,
	}

	// Sign the withdrawal
	sig, err := SignWithdraw(privateKey, testDomain, withdraw)
	require.NoError(t, err)
	assert.Len(t, sig, 65)

	// Verify the signature
	valid, err := VerifyWithdrawalSignature(testDomain, withdraw, sig)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyWithdrawalSignature_WrongAmount(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	withdraw := WithdrawData{
		Wallet:    wallet,
		Token:     "ETH",
		Amount:    big.NewInt(1000000000000000000),
		ToAddress: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
		Nonce:     1,
	}

	sig, err := SignWithdraw(privateKey, testDomain, withdraw)
	require.NoError(t, err)

	// Try to verify with a different amount
	withdraw.Amount = big.NewInt(2000000000000000000)
	valid, err := VerifyWithdrawalSignature(testDomain, withdraw, sig)
	assert.Error(t, err)
	assert.False(t, valid)
}

// =============================================================================
// Login Signature Tests
// =============================================================================

func TestSignAndVerifyLogin(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	login := LoginData{
		Wallet:    wallet,
		Message:   "Sign in to EidosExchange",
		Timestamp: time.Now().Unix(),
		Nonce:     1,
	}

	// Sign the login
	sig, err := SignLogin(privateKey, testDomain, login)
	require.NoError(t, err)
	assert.Len(t, sig, 65)

	// Verify the signature (no max age)
	valid, err := VerifyLoginSignature(testDomain, login, sig, 0)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyLoginSignature_Expired(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	login := LoginData{
		Wallet:    wallet,
		Message:   "Sign in to EidosExchange",
		Timestamp: time.Now().Add(-2 * time.Hour).Unix(), // 2 hours ago
		Nonce:     1,
	}

	sig, err := SignLogin(privateKey, testDomain, login)
	require.NoError(t, err)

	// Verify with 1 hour max age
	valid, err := VerifyLoginSignature(testDomain, login, sig, 3600)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureExpired)
	assert.False(t, valid)
}

func TestVerifyLoginSignature_FutureTimestamp(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	login := LoginData{
		Wallet:    wallet,
		Message:   "Sign in to EidosExchange",
		Timestamp: time.Now().Add(2 * time.Hour).Unix(), // 2 hours in the future
		Nonce:     1,
	}

	sig, err := SignLogin(privateKey, testDomain, login)
	require.NoError(t, err)

	// Verify with max age check enabled
	valid, err := VerifyLoginSignature(testDomain, login, sig, 3600)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "future")
	assert.False(t, valid)
}

func TestVerifyLoginSignature_NoMaxAge(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	login := LoginData{
		Wallet:    wallet,
		Message:   "Sign in to EidosExchange",
		Timestamp: time.Now().Add(-1000 * time.Hour).Unix(), // Very old
		Nonce:     1,
	}

	sig, err := SignLogin(privateKey, testDomain, login)
	require.NoError(t, err)

	// Verify with max age disabled (0)
	valid, err := VerifyLoginSignature(testDomain, login, sig, 0)
	require.NoError(t, err)
	assert.True(t, valid, "should not check expiration when maxAge is 0")
}

// =============================================================================
// Cancel Signature Tests
// =============================================================================

func TestSignAndVerifyCancel(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	cancel := CancelData{
		Wallet:  wallet,
		OrderID: "order-12345",
		Nonce:   1,
	}

	// Sign the cancel
	sig, err := SignCancel(privateKey, testDomain, cancel)
	require.NoError(t, err)
	assert.Len(t, sig, 65)

	// Verify the signature
	valid, err := VerifyCancelSignature(testDomain, cancel, sig)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyCancelSignature_WrongOrderID(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	cancel := CancelData{
		Wallet:  wallet,
		OrderID: "order-12345",
		Nonce:   1,
	}

	sig, err := SignCancel(privateKey, testDomain, cancel)
	require.NoError(t, err)

	// Try to verify with a different order ID
	cancel.OrderID = "order-different"
	valid, err := VerifyCancelSignature(testDomain, cancel, sig)
	assert.Error(t, err)
	assert.False(t, valid)
}

// =============================================================================
// Utility Function Tests
// =============================================================================

func TestParseSignature(t *testing.T) {
	// Generate a valid signature
	privateKey := getTestPrivateKey(t)
	hash := Keccak256([]byte("test"))
	sig, _ := crypto.Sign(hash, privateKey)
	sig[64] += 27
	sigHex := hex.EncodeToString(sig)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid with 0x prefix",
			input:   "0x" + sigHex,
			wantErr: false,
		},
		{
			name:    "valid without prefix",
			input:   sigHex,
			wantErr: false,
		},
		{
			name:    "too short",
			input:   "0x1234",
			wantErr: true,
		},
		{
			name:    "invalid hex",
			input:   "0xZZZZ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSignature(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, 65)
			}
		})
	}
}

func TestFormatSignature(t *testing.T) {
	sig := make([]byte, 65)
	for i := range sig {
		sig[i] = byte(i)
	}

	result := FormatSignature(sig)
	assert.True(t, strings.HasPrefix(result, "0x"))
	assert.Len(t, result, 132) // 2 for "0x" + 130 for 65 bytes in hex
}

func TestIsValidAddress(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected bool
	}{
		{
			name:     "valid address",
			addr:     "0x5FbDB2315678afecb367f032d93F642f64180aa3",
			expected: true,
		},
		{
			name:     "valid address lowercase",
			addr:     "0x5fbdb2315678afecb367f032d93f642f64180aa3",
			expected: true,
		},
		{
			name:     "valid zero address",
			addr:     "0x0000000000000000000000000000000000000000",
			expected: true,
		},
		{
			name:     "no 0x prefix",
			addr:     "5FbDB2315678afecb367f032d93F642f64180aa3",
			expected: false,
		},
		{
			name:     "too short",
			addr:     "0x5FbDB2315678afecb367f032d93F642f64180a",
			expected: false,
		},
		{
			name:     "too long",
			addr:     "0x5FbDB2315678afecb367f032d93F642f64180aa3a",
			expected: false,
		},
		{
			name:     "invalid hex character",
			addr:     "0x5FbDB2315678afecb367f032d93F642f64180aZZ",
			expected: false,
		},
		{
			name:     "empty string",
			addr:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAddress(tt.addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrivateKeyFromHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid with 0x prefix",
			input:   "0x" + testPrivateKeyHex,
			wantErr: false,
		},
		{
			name:    "valid without prefix",
			input:   testPrivateKeyHex,
			wantErr: false,
		},
		{
			name:    "invalid hex",
			input:   "ZZZZ",
			wantErr: true,
		},
		{
			name:    "too short",
			input:   "1234",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := PrivateKeyFromHex(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, key)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, key)
			}
		})
	}
}

func TestAddressFromPrivateKey(t *testing.T) {
	privateKey := getTestPrivateKey(t)
	address := AddressFromPrivateKey(privateKey)

	assert.True(t, strings.HasPrefix(address, "0x"))
	assert.Len(t, address, 42)
	assert.True(t, isValidAddress(address))
}

func TestAddressFromPrivateKey_Nil(t *testing.T) {
	address := AddressFromPrivateKey(nil)
	assert.Empty(t, address)
}

func TestPadLeft(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		size     int
		expected []byte
	}{
		{
			name:     "pad needed",
			input:    []byte{1, 2, 3},
			size:     5,
			expected: []byte{0, 0, 1, 2, 3},
		},
		{
			name:     "no pad needed - exact",
			input:    []byte{1, 2, 3},
			size:     3,
			expected: []byte{1, 2, 3},
		},
		{
			name:     "truncate needed",
			input:    []byte{1, 2, 3, 4, 5},
			size:     3,
			expected: []byte{3, 4, 5},
		},
		{
			name:     "empty input",
			input:    []byte{},
			size:     3,
			expected: []byte{0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padLeft(tt.input, tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHexToAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int // expected length
	}{
		{
			name:     "full address with prefix",
			input:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
			expected: 20,
		},
		{
			name:     "full address without prefix",
			input:    "5FbDB2315678afecb367f032d93F642f64180aa3",
			expected: 20,
		},
		{
			name:     "short address - should pad",
			input:    "0x1234",
			expected: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hexToAddress(tt.input)
			assert.Len(t, result, tt.expected)
		})
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestFullOrderFlow(t *testing.T) {
	// This test simulates a complete order signing and verification flow
	privateKey := getTestPrivateKey(t)
	wallet := AddressFromPrivateKey(privateKey)

	// Create an order
	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      uint8(OrderSideBuy),
		OrderType: uint8(OrderTypeLimit),
		Price:     big.NewInt(2000000000), // 2000 USDT
		Amount:    big.NewInt(1000000000000000000), // 1 ETH
		Nonce:     12345,
		Expiry:    time.Now().Add(1 * time.Hour).Unix(),
	}

	// Sign the order
	signature, err := SignOrder(privateKey, testDomain, order)
	require.NoError(t, err)

	// Format signature for transmission
	sigHex := FormatSignature(signature)
	assert.True(t, strings.HasPrefix(sigHex, "0x"))

	// Parse signature back
	parsedSig, err := ParseSignature(sigHex)
	require.NoError(t, err)

	// Verify the signature
	valid, err := VerifyOrderSignature(testDomain, order, parsedSig)
	require.NoError(t, err)
	assert.True(t, valid)

	// Verify that tampering is detected
	order.Amount = big.NewInt(2000000000000000000) // Try to double the amount
	valid, err = VerifyOrderSignature(testDomain, order, parsedSig)
	assert.Error(t, err)
	assert.False(t, valid)
}

func TestSignTypedData_NilPrivateKey(t *testing.T) {
	structHash := Keccak256([]byte("test"))
	_, err := SignTypedData(nil, testDomain, structHash)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private key cannot be nil")
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkKeccak256(b *testing.B) {
	data := []byte("test data for benchmarking keccak256 hash function")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Keccak256(data)
	}
}

func BenchmarkHashOrder(b *testing.B) {
	order := OrderData{
		Wallet:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    1700000000,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashOrder(order)
	}
}

func BenchmarkSignOrder(b *testing.B) {
	privateKey, _ := PrivateKeyFromHex(testPrivateKeyHex)
	wallet := AddressFromPrivateKey(privateKey)
	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    time.Now().Add(24 * time.Hour).Unix(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SignOrder(privateKey, testDomain, order)
	}
}

func BenchmarkVerifyOrderSignature(b *testing.B) {
	privateKey, _ := PrivateKeyFromHex(testPrivateKeyHex)
	wallet := AddressFromPrivateKey(privateKey)
	order := OrderData{
		Wallet:    wallet,
		Market:    "ETH-USDT",
		Side:      0,
		OrderType: 0,
		Price:     big.NewInt(1000000000),
		Amount:    big.NewInt(1000000000000000000),
		Nonce:     1,
		Expiry:    time.Now().Add(24 * time.Hour).Unix(),
	}
	sig, _ := SignOrder(privateKey, testDomain, order)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyOrderSignature(testDomain, order, sig)
	}
}

func BenchmarkRecoverAddress(b *testing.B) {
	privateKey, _ := PrivateKeyFromHex(testPrivateKeyHex)
	hash := Keccak256([]byte("test message"))
	sig, _ := crypto.Sign(hash, privateKey)
	sig[64] += 27
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecoverAddress(hash, sig)
	}
}
