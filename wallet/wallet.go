package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"fmt"

	"github.com/LeyouHong/samplechain/utils"
	"golang.org/x/crypto/ripemd160"
)

// Wallet 表示一个钱包（本质是密钥对）
// PrivateKey: 私钥（用于签名）
// PublicKey:  公钥（用于生成地址 / 验证签名）
//
// ⚠️ 注意：PrivateKey 使用 []byte 存储，而不是 ecdsa.PrivateKey
// 原因：gob 无法序列化 elliptic.Curve（内部是私有结构）
type Wallet struct {
	PrivateKey []byte
	PublicKey  []byte
}

// Base58Check 编码相关常量
const (
	checksumLength = 4          // 校验和长度（4字节）
	version        = byte(0x00) // 地址版本号（Bitcoin 主网是 0x00）
)

// MakeWallet 创建一个新的钱包（生成密钥对）
func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	return &Wallet{private, public}
}

// NewKeyPair 生成 ECDSA 密钥对
//
// 返回：
// - 私钥（序列化为 []byte，方便存储）
// - 公钥（X + Y 拼接）
//
// 使用 elliptic.P256 曲线（简化版，Bitcoin 实际用 secp256k1）
func NewKeyPair() ([]byte, []byte) {
	curve := elliptic.P256()

	// 生成 ECDSA 私钥
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	utils.Handle(err)

	// ⚠️ 关键点：将私钥转换为字节（否则 gob 无法序列化）
	privBytes, err := x509.MarshalECPrivateKey(private)
	utils.Handle(err)

	// 公钥由 X 和 Y 拼接而成
	// ECDSA 公钥不是一个“值”，而是一个点：
	// PublicKey = (X, Y)
	// 在椭圆曲线上：
	// X = 横坐标
	// Y = 纵坐标
	// 👉 本质是一个二维坐标点
	pub := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)

	return privBytes, pub
}

// Address 生成钱包地址（Bitcoin 风格）
//
// 流程：
// PublicKey
//
//	→ SHA256
//	→ RIPEMD160（得到 pubKeyHash）
//	→ 加 version（版本号）
//	→ 计算 checksum
//	→ 拼接 checksum
//	→ Base58 编码
//
// 返回：Base58 编码后的地址（[]byte）
func (w *Wallet) Address() []byte {
	// Step 1: 公钥哈希（Hash160）
	pubHash := PublicKeyHash(w.PublicKey)

	// Step 2: 加版本号（类似 Bitcoin address version）
	versionedHash := append([]byte{version}, pubHash...)

	// Step 3: 计算校验和
	checksum := Checksum(versionedHash)

	// Step 4: 拼接 version + pubHash + checksum
	fullHash := append(versionedHash, checksum...)

	// Step 5: Base58 编码（生成最终地址）
	address := Base58Encode(fullHash)

	// debug 输出（方便观察生成过程）
	fmt.Printf("pub key: %x\n", w.PublicKey)
	fmt.Printf("pub hash: %x\n", pubHash)
	fmt.Printf("address: %s\n", address)

	return address
}

// PublicKeyHash 对公钥做 Hash160（Bitcoin 标准做法）
//
// Hash160 = RIPEMD160(SHA256(pubKey))
//
// 为什么不用一次 hash？
// - SHA256：抗碰撞
// - RIPEMD160：缩短长度（20字节）
// → 组合更安全且更短
func PublicKeyHash(pubKey []byte) []byte {
	// Step 1: SHA256
	pubHash := sha256.Sum256(pubKey)

	// Step 2: RIPEMD160
	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	utils.Handle(err)

	return hasher.Sum(nil)
}

// Checksum 生成校验和（Base58Check 的一部分）
//
// 计算方式：
// checksum = SHA256(SHA256(payload)) 的前4字节
//
// 作用：
// 防止地址输入错误（例如输错字符）
func Checksum(payload []byte) []byte {
	firstHash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(firstHash[:])

	return secondHash[:checksumLength]
}

// BytesToPrivateKey 将 []byte 转回 ECDSA 私钥
//
// 使用场景：
// - 从文件加载钱包
// - 签名交易时恢复私钥
func BytesToPrivateKey(data []byte) *ecdsa.PrivateKey {
	key, err := x509.ParseECPrivateKey(data)
	utils.Handle(err)
	return key
}
