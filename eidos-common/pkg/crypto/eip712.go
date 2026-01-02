package crypto

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/sha3"
)

// EIP712Domain 域配置
type EIP712Domain struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	ChainID           int64  `json:"chainId"`
	VerifyingContract string `json:"verifyingContract"`
}

// DefaultDomain 默认域配置 (开发环境)
var DefaultDomain = EIP712Domain{
	Name:              "EidosExchange",
	Version:           "1",
	ChainID:           31337,
	VerifyingContract: "0x0000000000000000000000000000000000000000",
}

// IsMockMode 是否为 Mock 模式 (零地址)
func IsMockMode(domain EIP712Domain) bool {
	return domain.VerifyingContract == "0x0000000000000000000000000000000000000000"
}

// Keccak256 计算 Keccak256 哈希
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// Keccak256Hash 计算 Keccak256 哈希并返回 hex 字符串
func Keccak256Hash(data []byte) string {
	return "0x" + hex.EncodeToString(Keccak256(data))
}

// HashTypedDataDomain 计算域哈希
func HashTypedDataDomain(domain EIP712Domain) []byte {
	// EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)
	domainTypeHash := Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))

	nameHash := Keccak256([]byte(domain.Name))
	versionHash := Keccak256([]byte(domain.Version))
	chainID := big.NewInt(domain.ChainID)
	contract := hexToAddress(domain.VerifyingContract)

	// 拼接编码
	encoded := make([]byte, 0, 160)
	encoded = append(encoded, domainTypeHash...)
	encoded = append(encoded, nameHash...)
	encoded = append(encoded, versionHash...)
	encoded = append(encoded, padLeft(chainID.Bytes(), 32)...)
	encoded = append(encoded, padLeft(contract, 32)...)

	return Keccak256(encoded)
}

// OrderTypeHash Order 类型哈希
var OrderTypeHash = Keccak256([]byte("Order(address wallet,string market,uint8 side,uint8 orderType,uint256 price,uint256 amount,uint256 nonce,uint256 expiry)"))

// CancelTypeHash Cancel 类型哈希
var CancelTypeHash = Keccak256([]byte("Cancel(address wallet,string orderId,uint256 nonce)"))

// WithdrawTypeHash Withdraw 类型哈希
var WithdrawTypeHash = Keccak256([]byte("Withdraw(address wallet,string token,uint256 amount,string toAddress,uint256 nonce)"))

// OrderData 订单数据
type OrderData struct {
	Wallet    string
	Market    string
	Side      uint8
	OrderType uint8
	Price     *big.Int
	Amount    *big.Int
	Nonce     uint64
	Expiry    int64
}

// HashOrder 计算订单结构哈希
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

// HashTypedDataV4 计算 EIP-712 TypedData V4 哈希
func HashTypedDataV4(domain EIP712Domain, structHash []byte) []byte {
	domainSeparator := HashTypedDataDomain(domain)

	// \x19\x01 + domainSeparator + structHash
	encoded := make([]byte, 0, 66)
	encoded = append(encoded, 0x19, 0x01)
	encoded = append(encoded, domainSeparator...)
	encoded = append(encoded, structHash...)

	return Keccak256(encoded)
}

// RecoverAddress 从签名恢复地址
func RecoverAddress(hash, signature []byte) (string, error) {
	if len(signature) != 65 {
		return "", fmt.Errorf("invalid signature length: %d", len(signature))
	}

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	v := signature[64]

	// 调整 v 值
	if v >= 27 {
		v -= 27
	}

	// 使用 secp256k1 恢复公钥
	pubKey, err := recoverPubkey(hash, r, s, v)
	if err != nil {
		return "", err
	}

	// 公钥转地址
	address := pubkeyToAddress(pubKey)
	return address, nil
}

// VerifySignature 验证签名
func VerifySignature(wallet string, hash, signature []byte) (bool, error) {
	recovered, err := RecoverAddress(hash, signature)
	if err != nil {
		return false, err
	}

	return strings.EqualFold(recovered, wallet), nil
}

// 辅助函数

func hexToAddress(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	if len(s) < 40 {
		s = strings.Repeat("0", 40-len(s)) + s
	}
	b, _ := hex.DecodeString(s)
	return b
}

func padLeft(data []byte, size int) []byte {
	if len(data) >= size {
		return data[len(data)-size:]
	}
	result := make([]byte, size)
	copy(result[size-len(data):], data)
	return result
}

func pubkeyToAddress(pub *ecdsa.PublicKey) string {
	pubBytes := make([]byte, 64)
	copy(pubBytes[:32], padLeft(pub.X.Bytes(), 32))
	copy(pubBytes[32:], padLeft(pub.Y.Bytes(), 32))

	hash := Keccak256(pubBytes)
	return "0x" + hex.EncodeToString(hash[12:])
}

// recoverPubkey 简化的公钥恢复 (实际项目应使用 go-ethereum 或专门的加密库)
func recoverPubkey(hash []byte, r, s *big.Int, v byte) (*ecdsa.PublicKey, error) {
	// 注意: 这是简化实现，实际项目应使用 go-ethereum/crypto 包
	// import "github.com/ethereum/go-ethereum/crypto"
	// return crypto.Ecrecover(hash, signature)
	return nil, fmt.Errorf("please use go-ethereum/crypto for production")
}
