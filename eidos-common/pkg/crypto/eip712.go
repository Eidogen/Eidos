// Package crypto provides EIP-712 typed data signing and verification utilities.
// EIP-712 is a standard for hashing and signing typed structured data, commonly used
// in Ethereum for secure off-chain message signing with on-chain verification.
//
// This package implements:
//   - EIP-712 domain separator calculation
//   - Typed data hashing (Order, Withdrawal, Login, Cancel)
//   - Signature verification using secp256k1 curve
//   - Address recovery from signatures
//
// Reference: https://eips.ethereum.org/EIPS/eip-712
package crypto

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)

// =============================================================================
// Error Definitions
// =============================================================================

var (
	// ErrInvalidSignatureLength indicates the signature is not 65 bytes.
	ErrInvalidSignatureLength = errors.New("invalid signature length: expected 65 bytes")

	// ErrInvalidSignatureFormat indicates the signature format is malformed.
	ErrInvalidSignatureFormat = errors.New("invalid signature format")

	// ErrSignatureRecoveryFailed indicates public key recovery from signature failed.
	ErrSignatureRecoveryFailed = errors.New("failed to recover public key from signature")

	// ErrAddressMismatch indicates the recovered address does not match the expected address.
	ErrAddressMismatch = errors.New("signature address mismatch")

	// ErrInvalidAddress indicates an invalid Ethereum address format.
	ErrInvalidAddress = errors.New("invalid ethereum address format")

	// ErrSignatureExpired indicates the signature has expired.
	ErrSignatureExpired = errors.New("signature has expired")

	// ErrInvalidNonce indicates an invalid nonce value.
	ErrInvalidNonce = errors.New("invalid nonce")

	// ErrNilAmount indicates a nil amount was provided.
	ErrNilAmount = errors.New("amount cannot be nil")

	// ErrNilPrice indicates a nil price was provided.
	ErrNilPrice = errors.New("price cannot be nil")

	// ErrEmptyMarket indicates an empty market string.
	ErrEmptyMarket = errors.New("market cannot be empty")

	// ErrEmptyToken indicates an empty token string.
	ErrEmptyToken = errors.New("token cannot be empty")

	// ErrEmptyMessage indicates an empty message string.
	ErrEmptyMessage = errors.New("message cannot be empty")
)

// =============================================================================
// Domain Configuration
// =============================================================================

// EIP712Domain represents the EIP-712 domain separator parameters.
// The domain separator is used to prevent signature replay attacks across
// different contracts, chains, or applications.
type EIP712Domain struct {
	// Name is the user-readable name of the signing domain (e.g., "EidosExchange").
	Name string `json:"name"`

	// Version is the current major version of the signing domain.
	Version string `json:"version"`

	// ChainID is the EIP-155 chain ID (e.g., 1 for Ethereum mainnet, 31337 for local).
	ChainID int64 `json:"chainId"`

	// VerifyingContract is the address of the contract that will verify the signature.
	VerifyingContract string `json:"verifyingContract"`
}

// DefaultDomain provides a default domain configuration for development environments.
// In production, this should be replaced with actual contract addresses.
var DefaultDomain = EIP712Domain{
	Name:              "EidosExchange",
	Version:           "1",
	ChainID:           31337,
	VerifyingContract: "0x0000000000000000000000000000000000000000",
}

// IsMockMode returns true if the domain is configured with a zero address,
// indicating a mock or development environment.
func IsMockMode(domain EIP712Domain) bool {
	return domain.VerifyingContract == "0x0000000000000000000000000000000000000000"
}

// Validate checks if the domain configuration is valid.
func (d *EIP712Domain) Validate() error {
	if d.Name == "" {
		return errors.New("domain name cannot be empty")
	}
	if d.Version == "" {
		return errors.New("domain version cannot be empty")
	}
	if d.ChainID <= 0 {
		return errors.New("chain ID must be positive")
	}
	if !isValidAddress(d.VerifyingContract) {
		return ErrInvalidAddress
	}
	return nil
}

// =============================================================================
// Type Hashes (Pre-computed for efficiency)
// =============================================================================

// DomainTypeHash is the keccak256 hash of the EIP712Domain type string.
var DomainTypeHash = Keccak256([]byte(
	"EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)",
))

// OrderTypeHash is the keccak256 hash of the Order type string.
// Order represents a trading order with all necessary parameters.
var OrderTypeHash = Keccak256([]byte(
	"Order(address wallet,string market,uint8 side,uint8 orderType,uint256 price,uint256 amount,uint256 nonce,uint256 expiry)",
))

// CancelTypeHash is the keccak256 hash of the Cancel type string.
// Cancel represents an order cancellation request.
var CancelTypeHash = Keccak256([]byte(
	"Cancel(address wallet,string orderId,uint256 nonce)",
))

// WithdrawTypeHash is the keccak256 hash of the Withdraw type string.
// Withdraw represents a withdrawal request from the exchange.
var WithdrawTypeHash = Keccak256([]byte(
	"Withdraw(address wallet,string token,uint256 amount,string toAddress,uint256 nonce)",
))

// LoginTypeHash is the keccak256 hash of the Login type string.
// Login represents an authentication request.
var LoginTypeHash = Keccak256([]byte(
	"Login(address wallet,string message,uint256 timestamp,uint256 nonce)",
))

// =============================================================================
// Data Structures
// =============================================================================

// OrderSide represents the side of an order (buy or sell).
type OrderSide uint8

const (
	// OrderSideBuy represents a buy order.
	OrderSideBuy OrderSide = 0
	// OrderSideSell represents a sell order.
	OrderSideSell OrderSide = 1
)

// OrderType represents the type of order.
type OrderType uint8

const (
	// OrderTypeLimit represents a limit order.
	OrderTypeLimit OrderType = 0
	// OrderTypeMarket represents a market order.
	OrderTypeMarket OrderType = 1
)

// OrderData represents the data structure for a trading order.
type OrderData struct {
	// Wallet is the Ethereum address of the order owner.
	Wallet string

	// Market is the trading pair identifier (e.g., "ETH-USDT").
	Market string

	// Side indicates whether this is a buy (0) or sell (1) order.
	Side uint8

	// OrderType indicates the order type: 0 for limit, 1 for market.
	OrderType uint8

	// Price is the order price in the smallest unit (e.g., wei).
	// For market orders, this may be ignored.
	Price *big.Int

	// Amount is the order quantity in the smallest unit.
	Amount *big.Int

	// Nonce is a unique number to prevent replay attacks.
	Nonce uint64

	// Expiry is the Unix timestamp after which the order is invalid.
	Expiry int64
}

// Validate checks if the order data is valid.
func (o *OrderData) Validate() error {
	if !isValidAddress(o.Wallet) {
		return ErrInvalidAddress
	}
	if o.Market == "" {
		return ErrEmptyMarket
	}
	if o.Side > 1 {
		return errors.New("invalid order side: must be 0 (buy) or 1 (sell)")
	}
	if o.OrderType > 1 {
		return errors.New("invalid order type: must be 0 (limit) or 1 (market)")
	}
	if o.Price == nil {
		return ErrNilPrice
	}
	if o.Amount == nil {
		return ErrNilAmount
	}
	if o.Amount.Sign() <= 0 {
		return errors.New("amount must be positive")
	}
	return nil
}

// CancelData represents the data structure for an order cancellation.
type CancelData struct {
	// Wallet is the Ethereum address of the order owner.
	Wallet string

	// OrderID is the unique identifier of the order to cancel.
	OrderID string

	// Nonce is a unique number to prevent replay attacks.
	Nonce uint64
}

// Validate checks if the cancel data is valid.
func (c *CancelData) Validate() error {
	if !isValidAddress(c.Wallet) {
		return ErrInvalidAddress
	}
	if c.OrderID == "" {
		return errors.New("order ID cannot be empty")
	}
	return nil
}

// WithdrawData represents the data structure for a withdrawal request.
type WithdrawData struct {
	// Wallet is the Ethereum address initiating the withdrawal.
	Wallet string

	// Token is the token symbol or contract address to withdraw.
	Token string

	// Amount is the withdrawal amount in the smallest unit.
	Amount *big.Int

	// ToAddress is the destination address for the withdrawal.
	ToAddress string

	// Nonce is a unique number to prevent replay attacks.
	Nonce uint64
}

// Validate checks if the withdrawal data is valid.
func (w *WithdrawData) Validate() error {
	if !isValidAddress(w.Wallet) {
		return fmt.Errorf("invalid wallet address: %w", ErrInvalidAddress)
	}
	if w.Token == "" {
		return ErrEmptyToken
	}
	if w.Amount == nil {
		return ErrNilAmount
	}
	if w.Amount.Sign() <= 0 {
		return errors.New("withdrawal amount must be positive")
	}
	if w.ToAddress == "" {
		return errors.New("destination address cannot be empty")
	}
	return nil
}

// LoginData represents the data structure for a login/authentication request.
type LoginData struct {
	// Wallet is the Ethereum address attempting to authenticate.
	Wallet string

	// Message is a custom message to sign (e.g., "Sign in to EidosExchange").
	Message string

	// Timestamp is the Unix timestamp when the login was initiated.
	Timestamp int64

	// Nonce is a unique number to prevent replay attacks.
	Nonce uint64
}

// Validate checks if the login data is valid.
func (l *LoginData) Validate() error {
	if !isValidAddress(l.Wallet) {
		return ErrInvalidAddress
	}
	if l.Message == "" {
		return ErrEmptyMessage
	}
	if l.Timestamp <= 0 {
		return errors.New("timestamp must be positive")
	}
	return nil
}

// =============================================================================
// Hashing Functions
// =============================================================================

// Keccak256 computes the Keccak-256 hash of the input data.
// This is the hash function used throughout Ethereum and EIP-712.
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// Keccak256Hash computes the Keccak-256 hash and returns it as a hex string
// with "0x" prefix.
func Keccak256Hash(data []byte) string {
	return "0x" + hex.EncodeToString(Keccak256(data))
}

// HashTypedDataDomain computes the EIP-712 domain separator hash.
// The domain separator is unique to each signing domain and prevents
// signature replay across different applications.
func HashTypedDataDomain(domain EIP712Domain) []byte {
	nameHash := Keccak256([]byte(domain.Name))
	versionHash := Keccak256([]byte(domain.Version))
	chainID := big.NewInt(domain.ChainID)
	contract := hexToAddress(domain.VerifyingContract)

	// Encode according to EIP-712: hashStruct(EIP712Domain)
	// = keccak256(typeHash || encodeData(s))
	encoded := make([]byte, 0, 160)
	encoded = append(encoded, DomainTypeHash...)
	encoded = append(encoded, nameHash...)
	encoded = append(encoded, versionHash...)
	encoded = append(encoded, padLeft(chainID.Bytes(), 32)...)
	encoded = append(encoded, padLeft(contract, 32)...)

	return Keccak256(encoded)
}

// HashOrder computes the struct hash for an Order.
// This follows the EIP-712 encoding rules for structured data.
func HashOrder(order OrderData) []byte {
	encoded := make([]byte, 0, 288)
	encoded = append(encoded, OrderTypeHash...)
	encoded = append(encoded, padLeft(hexToAddress(order.Wallet), 32)...)
	encoded = append(encoded, Keccak256([]byte(order.Market))...)
	encoded = append(encoded, padLeft([]byte{order.Side}, 32)...)
	encoded = append(encoded, padLeft([]byte{order.OrderType}, 32)...)
	encoded = append(encoded, padLeft(order.Price.Bytes(), 32)...)
	encoded = append(encoded, padLeft(order.Amount.Bytes(), 32)...)
	encoded = append(encoded, padLeft(big.NewInt(int64(order.Nonce)).Bytes(), 32)...)
	encoded = append(encoded, padLeft(big.NewInt(order.Expiry).Bytes(), 32)...)

	return Keccak256(encoded)
}

// HashCancel computes the struct hash for a Cancel request.
func HashCancel(cancel CancelData) []byte {
	encoded := make([]byte, 0, 128)
	encoded = append(encoded, CancelTypeHash...)
	encoded = append(encoded, padLeft(hexToAddress(cancel.Wallet), 32)...)
	encoded = append(encoded, Keccak256([]byte(cancel.OrderID))...)
	encoded = append(encoded, padLeft(big.NewInt(int64(cancel.Nonce)).Bytes(), 32)...)

	return Keccak256(encoded)
}

// HashWithdraw computes the struct hash for a Withdraw request.
func HashWithdraw(withdraw WithdrawData) []byte {
	encoded := make([]byte, 0, 192)
	encoded = append(encoded, WithdrawTypeHash...)
	encoded = append(encoded, padLeft(hexToAddress(withdraw.Wallet), 32)...)
	encoded = append(encoded, Keccak256([]byte(withdraw.Token))...)
	encoded = append(encoded, padLeft(withdraw.Amount.Bytes(), 32)...)
	encoded = append(encoded, Keccak256([]byte(withdraw.ToAddress))...)
	encoded = append(encoded, padLeft(big.NewInt(int64(withdraw.Nonce)).Bytes(), 32)...)

	return Keccak256(encoded)
}

// HashLogin computes the struct hash for a Login request.
func HashLogin(login LoginData) []byte {
	encoded := make([]byte, 0, 160)
	encoded = append(encoded, LoginTypeHash...)
	encoded = append(encoded, padLeft(hexToAddress(login.Wallet), 32)...)
	encoded = append(encoded, Keccak256([]byte(login.Message))...)
	encoded = append(encoded, padLeft(big.NewInt(login.Timestamp).Bytes(), 32)...)
	encoded = append(encoded, padLeft(big.NewInt(int64(login.Nonce)).Bytes(), 32)...)

	return Keccak256(encoded)
}

// HashTypedDataV4 computes the final EIP-712 hash for signing.
// This combines the domain separator with the struct hash using the
// standard prefix "\x19\x01".
//
// The resulting hash can be signed using eth_sign or similar methods.
func HashTypedDataV4(domain EIP712Domain, structHash []byte) []byte {
	domainSeparator := HashTypedDataDomain(domain)

	// EIP-712 encoding: "\x19\x01" || domainSeparator || hashStruct(message)
	encoded := make([]byte, 0, 66)
	encoded = append(encoded, 0x19, 0x01)
	encoded = append(encoded, domainSeparator...)
	encoded = append(encoded, structHash...)

	return Keccak256(encoded)
}

// =============================================================================
// Signature Recovery and Verification
// =============================================================================

// RecoverAddress recovers the Ethereum address that created the signature
// for the given hash. The signature must be 65 bytes in [R || S || V] format.
//
// The function handles both legacy (V = 27/28) and modern (V = 0/1) signature formats.
//
// Parameters:
//   - hash: The 32-byte message hash that was signed
//   - signature: The 65-byte signature in [R || S || V] format
//
// Returns:
//   - The recovered Ethereum address as a checksummed hex string
//   - An error if recovery fails
func RecoverAddress(hash, signature []byte) (string, error) {
	// Validate hash length
	if len(hash) != 32 {
		return "", fmt.Errorf("invalid hash length: expected 32, got %d", len(hash))
	}

	// Validate signature length
	if len(signature) != 65 {
		return "", ErrInvalidSignatureLength
	}

	// Make a copy to avoid modifying the original signature
	sig := make([]byte, 65)
	copy(sig, signature)

	// Normalize V value to 0 or 1
	// Ethereum uses V values of 27/28 for legacy compatibility,
	// but the underlying crypto library expects 0/1
	v := sig[64]
	if v >= 27 {
		sig[64] = v - 27
	}

	// Ensure V is valid (0 or 1)
	if sig[64] > 1 {
		return "", fmt.Errorf("%w: invalid V value %d", ErrInvalidSignatureFormat, v)
	}

	// Use go-ethereum's crypto package for ECDSA recovery
	pubKey, err := crypto.Ecrecover(hash, sig)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSignatureRecoveryFailed, err)
	}

	// Validate recovered public key length
	if len(pubKey) != 65 {
		return "", fmt.Errorf("%w: invalid public key length", ErrSignatureRecoveryFailed)
	}

	// Convert uncompressed public key to Ethereum address
	// Address = keccak256(pubKey[1:])[12:]
	pubKeyHash := Keccak256(pubKey[1:])
	address := "0x" + hex.EncodeToString(pubKeyHash[12:])

	return strings.ToLower(address), nil
}

// RecoverAddressFromTypedData is a convenience function that combines
// hash computation and address recovery for EIP-712 typed data.
func RecoverAddressFromTypedData(domain EIP712Domain, structHash, signature []byte) (string, error) {
	hash := HashTypedDataV4(domain, structHash)
	return RecoverAddress(hash, signature)
}

// VerifySignature verifies that a signature was created by the specified wallet address.
//
// Parameters:
//   - wallet: The expected signer's Ethereum address
//   - hash: The 32-byte message hash that was signed
//   - signature: The 65-byte signature
//
// Returns:
//   - true if the signature is valid and matches the wallet address
//   - An error if verification fails
func VerifySignature(wallet string, hash, signature []byte) (bool, error) {
	// Validate wallet address format
	if !isValidAddress(wallet) {
		return false, ErrInvalidAddress
	}

	// Recover the signer's address
	recovered, err := RecoverAddress(hash, signature)
	if err != nil {
		return false, fmt.Errorf("signature verification failed: %w", err)
	}

	// Compare addresses (case-insensitive)
	if !strings.EqualFold(recovered, wallet) {
		return false, ErrAddressMismatch
	}

	return true, nil
}

// =============================================================================
// High-Level Verification Functions
// =============================================================================

// VerifyOrderSignature verifies an order signature against the provided order data.
// It validates the order data, computes the EIP-712 hash, and verifies the signature.
//
// Parameters:
//   - domain: The EIP-712 domain configuration
//   - order: The order data that was signed
//   - signature: The 65-byte signature
//
// Returns:
//   - true if the signature is valid
//   - An error describing why verification failed, or nil on success
func VerifyOrderSignature(domain EIP712Domain, order OrderData, signature []byte) (bool, error) {
	// Validate domain configuration
	if err := domain.Validate(); err != nil {
		return false, fmt.Errorf("invalid domain: %w", err)
	}

	// Validate order data
	if err := order.Validate(); err != nil {
		return false, fmt.Errorf("invalid order: %w", err)
	}

	// Check if order has expired
	if order.Expiry > 0 && time.Now().Unix() > order.Expiry {
		return false, ErrSignatureExpired
	}

	// Compute the order struct hash
	structHash := HashOrder(order)

	// Compute the final EIP-712 hash
	hash := HashTypedDataV4(domain, structHash)

	// Verify the signature
	return VerifySignature(order.Wallet, hash, signature)
}

// VerifyWithdrawalSignature verifies a withdrawal signature against the provided data.
// It validates the withdrawal data, computes the EIP-712 hash, and verifies the signature.
//
// Parameters:
//   - domain: The EIP-712 domain configuration
//   - withdrawal: The withdrawal data that was signed
//   - signature: The 65-byte signature
//
// Returns:
//   - true if the signature is valid
//   - An error describing why verification failed, or nil on success
func VerifyWithdrawalSignature(domain EIP712Domain, withdrawal WithdrawData, signature []byte) (bool, error) {
	// Validate domain configuration
	if err := domain.Validate(); err != nil {
		return false, fmt.Errorf("invalid domain: %w", err)
	}

	// Validate withdrawal data
	if err := withdrawal.Validate(); err != nil {
		return false, fmt.Errorf("invalid withdrawal: %w", err)
	}

	// Compute the withdrawal struct hash
	structHash := HashWithdraw(withdrawal)

	// Compute the final EIP-712 hash
	hash := HashTypedDataV4(domain, structHash)

	// Verify the signature
	return VerifySignature(withdrawal.Wallet, hash, signature)
}

// VerifyLoginSignature verifies a login signature against the provided data.
// It validates the login data, checks expiration, and verifies the signature.
//
// Parameters:
//   - domain: The EIP-712 domain configuration
//   - login: The login data that was signed
//   - signature: The 65-byte signature
//   - maxAge: Maximum age of the signature in seconds (0 to disable expiry check)
//
// Returns:
//   - true if the signature is valid
//   - An error describing why verification failed, or nil on success
func VerifyLoginSignature(domain EIP712Domain, login LoginData, signature []byte, maxAge int64) (bool, error) {
	// Validate domain configuration
	if err := domain.Validate(); err != nil {
		return false, fmt.Errorf("invalid domain: %w", err)
	}

	// Validate login data
	if err := login.Validate(); err != nil {
		return false, fmt.Errorf("invalid login: %w", err)
	}

	// Check if login signature has expired
	if maxAge > 0 {
		age := time.Now().Unix() - login.Timestamp
		if age > maxAge {
			return false, fmt.Errorf("%w: signature is %d seconds old, max allowed is %d",
				ErrSignatureExpired, age, maxAge)
		}
		// Also check for signatures from the future (clock skew tolerance: 60 seconds)
		if age < -60 {
			return false, fmt.Errorf("signature timestamp is in the future")
		}
	}

	// Compute the login struct hash
	structHash := HashLogin(login)

	// Compute the final EIP-712 hash
	hash := HashTypedDataV4(domain, structHash)

	// Verify the signature
	return VerifySignature(login.Wallet, hash, signature)
}

// VerifyCancelSignature verifies a cancel order signature.
//
// Parameters:
//   - domain: The EIP-712 domain configuration
//   - cancel: The cancel data that was signed
//   - signature: The 65-byte signature
//
// Returns:
//   - true if the signature is valid
//   - An error describing why verification failed, or nil on success
func VerifyCancelSignature(domain EIP712Domain, cancel CancelData, signature []byte) (bool, error) {
	// Validate domain configuration
	if err := domain.Validate(); err != nil {
		return false, fmt.Errorf("invalid domain: %w", err)
	}

	// Validate cancel data
	if err := cancel.Validate(); err != nil {
		return false, fmt.Errorf("invalid cancel: %w", err)
	}

	// Compute the cancel struct hash
	structHash := HashCancel(cancel)

	// Compute the final EIP-712 hash
	hash := HashTypedDataV4(domain, structHash)

	// Verify the signature
	return VerifySignature(cancel.Wallet, hash, signature)
}

// =============================================================================
// Signing Functions (for testing and client-side use)
// =============================================================================

// SignTypedData signs EIP-712 typed data with a private key.
// This is primarily useful for testing and client-side applications.
//
// Parameters:
//   - privateKey: The ECDSA private key
//   - domain: The EIP-712 domain configuration
//   - structHash: The pre-computed struct hash
//
// Returns:
//   - The 65-byte signature in [R || S || V] format
//   - An error if signing fails
func SignTypedData(privateKey *ecdsa.PrivateKey, domain EIP712Domain, structHash []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, errors.New("private key cannot be nil")
	}

	// Compute the final hash
	hash := HashTypedDataV4(domain, structHash)

	// Sign the hash
	sig, err := crypto.Sign(hash, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Convert V from 0/1 to 27/28 for Ethereum compatibility
	sig[64] += 27

	return sig, nil
}

// SignOrder signs an order with a private key.
func SignOrder(privateKey *ecdsa.PrivateKey, domain EIP712Domain, order OrderData) ([]byte, error) {
	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("invalid order: %w", err)
	}
	structHash := HashOrder(order)
	return SignTypedData(privateKey, domain, structHash)
}

// SignWithdraw signs a withdrawal request with a private key.
func SignWithdraw(privateKey *ecdsa.PrivateKey, domain EIP712Domain, withdraw WithdrawData) ([]byte, error) {
	if err := withdraw.Validate(); err != nil {
		return nil, fmt.Errorf("invalid withdrawal: %w", err)
	}
	structHash := HashWithdraw(withdraw)
	return SignTypedData(privateKey, domain, structHash)
}

// SignLogin signs a login request with a private key.
func SignLogin(privateKey *ecdsa.PrivateKey, domain EIP712Domain, login LoginData) ([]byte, error) {
	if err := login.Validate(); err != nil {
		return nil, fmt.Errorf("invalid login: %w", err)
	}
	structHash := HashLogin(login)
	return SignTypedData(privateKey, domain, structHash)
}

// SignCancel signs a cancel request with a private key.
func SignCancel(privateKey *ecdsa.PrivateKey, domain EIP712Domain, cancel CancelData) ([]byte, error) {
	if err := cancel.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cancel: %w", err)
	}
	structHash := HashCancel(cancel)
	return SignTypedData(privateKey, domain, structHash)
}

// =============================================================================
// Utility Functions
// =============================================================================

// ParseSignature parses a hex-encoded signature string into bytes.
// Accepts signatures with or without "0x" prefix.
func ParseSignature(sigHex string) ([]byte, error) {
	sigHex = strings.TrimPrefix(sigHex, "0x")
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}
	if len(sig) != 65 {
		return nil, ErrInvalidSignatureLength
	}
	return sig, nil
}

// FormatSignature formats a signature as a hex string with "0x" prefix.
func FormatSignature(sig []byte) string {
	return "0x" + hex.EncodeToString(sig)
}

// hexToAddress converts a hex address string to a 20-byte array.
// Handles addresses with or without "0x" prefix.
func hexToAddress(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	// Pad short addresses with leading zeros
	if len(s) < 40 {
		s = strings.Repeat("0", 40-len(s)) + s
	}
	// Truncate long addresses
	if len(s) > 40 {
		s = s[len(s)-40:]
	}
	b, _ := hex.DecodeString(s)
	return b
}

// padLeft pads a byte slice with leading zeros to reach the specified size.
// If the input is longer than size, it returns the rightmost 'size' bytes.
func padLeft(data []byte, size int) []byte {
	if len(data) >= size {
		return data[len(data)-size:]
	}
	result := make([]byte, size)
	copy(result[size-len(data):], data)
	return result
}

// isValidAddress checks if a string is a valid Ethereum address.
// Valid addresses are 42 characters long (including "0x" prefix) and contain only hex characters.
func isValidAddress(addr string) bool {
	if !strings.HasPrefix(addr, "0x") {
		return false
	}
	if len(addr) != 42 {
		return false
	}
	_, err := hex.DecodeString(addr[2:])
	return err == nil
}

// pubkeyToAddress converts an ECDSA public key to an Ethereum address.
func pubkeyToAddress(pub *ecdsa.PublicKey) string {
	if pub == nil {
		return ""
	}
	pubBytes := make([]byte, 64)
	copy(pubBytes[:32], padLeft(pub.X.Bytes(), 32))
	copy(pubBytes[32:], padLeft(pub.Y.Bytes(), 32))

	hash := Keccak256(pubBytes)
	return "0x" + hex.EncodeToString(hash[12:])
}

// AddressFromPrivateKey derives the Ethereum address from a private key.
func AddressFromPrivateKey(privateKey *ecdsa.PrivateKey) string {
	if privateKey == nil {
		return ""
	}
	return pubkeyToAddress(&privateKey.PublicKey)
}

// PrivateKeyFromHex parses a hex-encoded private key.
// Accepts keys with or without "0x" prefix.
func PrivateKeyFromHex(hexKey string) (*ecdsa.PrivateKey, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	return privateKey, nil
}

// =============================================================================
// Personal Sign (EIP-191) Support
// =============================================================================

// PersonalSignPrefix is the Ethereum personal sign prefix
const PersonalSignPrefix = "\x19Ethereum Signed Message:\n"

// HashPersonalMessage hashes a message with the Ethereum personal sign prefix (EIP-191).
// This creates the hash that would be signed by eth_sign / personal_sign.
func HashPersonalMessage(message []byte) []byte {
	// Format: "\x19Ethereum Signed Message:\n" + len(message) + message
	prefix := fmt.Sprintf("%s%d", PersonalSignPrefix, len(message))
	data := append([]byte(prefix), message...)
	return Keccak256(data)
}

// VerifyPersonalSignature verifies an Ethereum personal signature (EIP-191).
// This is used for simple message signing, not EIP-712 typed data.
//
// Parameters:
//   - wallet: The expected signer's Ethereum address
//   - messageHash: The raw message hash (NOT prefixed - this function will add the prefix)
//   - signature: The 65-byte signature
//
// Returns:
//   - true if the signature is valid and matches the wallet address
//   - An error if verification fails
func VerifyPersonalSignature(wallet string, messageHash, signature []byte) (bool, error) {
	// Validate wallet address format
	if !isValidAddress(wallet) {
		return false, ErrInvalidAddress
	}

	// Hash with personal sign prefix
	prefixedHash := HashPersonalMessage(messageHash)

	// Recover the signer's address
	recovered, err := RecoverAddress(prefixedHash, signature)
	if err != nil {
		return false, fmt.Errorf("personal signature verification failed: %w", err)
	}

	// Compare addresses (case-insensitive)
	if !strings.EqualFold(recovered, wallet) {
		return false, ErrAddressMismatch
	}

	return true, nil
}
