package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"fmt"

	"github.com/LeyouHong/samplechain/utils"
	"golang.org/x/crypto/ripemd160"
)

/////////////////////////////////////////////////////////////////
// 🧠 Wallet 本质是什么？
//
// Wallet ≠ 钱包应用
// Wallet = 密钥对（PrivateKey + PublicKey）
/////////////////////////////////////////////////////////////////

// Wallet 表示一个“账户体系”
//
// 👉 本质：一对非对称加密密钥
// - PrivateKey：签名用（证明你是你）
// - PublicKey：验证 + 生成地址用
//
// ⚠️ 为什么是 []byte？
// 因为 ecdsa.PrivateKey 不能直接序列化（gob / 文件存储）
type Wallet struct {
	PrivateKey []byte // 🔐 私钥（经过序列化）
	PublicKey  []byte // 🔑 公钥（X + Y 拼接）
}

/////////////////////////////////////////////////////////////////
// 🔢 地址系统相关常量
/////////////////////////////////////////////////////////////////

const (
	ChecksumLength = 4          // 🧾 校验和长度（防止输入错误）
	Version        = byte(0x00) // 🌐 地址版本（类似 Bitcoin mainnet）
)

/////////////////////////////////////////////////////////////////
// 🏭 创建钱包（生成密钥对）
/////////////////////////////////////////////////////////////////

func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	return &Wallet{private, public}
}

/////////////////////////////////////////////////////////////////
// 🔍 地址合法性校验
//
// 用于：
// - 防止用户输入错误地址
// - 防止篡改
/////////////////////////////////////////////////////////////////

func ValidateAddress(address string) bool {

	// Step 1: Base58 解码地址
	pubKeyHash := Base58Decode([]byte(address))

	// Step 2: 提取 checksum（最后 4 bytes）
	actualChecksum := pubKeyHash[len(pubKeyHash)-ChecksumLength:]

	// Step 3: 提取 version
	version := pubKeyHash[0]

	// Step 4: 提取 pubKeyHash（去掉 version + checksum）
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-ChecksumLength]

	// Step 5: 重新计算 checksum
	targetChecksum := Checksum(append([]byte{version}, pubKeyHash...))

	// Step 6: 比较
	return bytes.Equal(actualChecksum, targetChecksum)
}

/////////////////////////////////////////////////////////////////
// 🔑 生成密钥对（核心密码学）
/////////////////////////////////////////////////////////////////

func NewKeyPair() ([]byte, []byte) {

	// 使用椭圆曲线（ECDSA）
	curve := elliptic.P256()

	// Step 1: 生成 ECDSA 私钥
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	utils.Handle(err)

	/////////////////////////////////////////////////////////////////
	// ⚠️ 为什么要 marshal？
	//
	// ecdsa.PrivateKey 不能直接存储：
	// - 内部包含复杂结构（Curve）
	// - gob / JSON 无法序列化
	//
	// 👉 所以转成 []byte 存
	/////////////////////////////////////////////////////////////////

	privBytes, err := x509.MarshalECPrivateKey(private)
	utils.Handle(err)

	/////////////////////////////////////////////////////////////////
	// 🔑 公钥 = 椭圆曲线上的一个点
	//
	// 公钥不是一个数，而是：
	// (X, Y)
	//
	// 👉 本质：曲线上的坐标点
	/////////////////////////////////////////////////////////////////

	pub := append(
		private.PublicKey.X.Bytes(),
		private.PublicKey.Y.Bytes()...,
	)

	return privBytes, pub
}

/////////////////////////////////////////////////////////////////
// 🏠 地址生成（Bitcoin 风格）
//
// Wallet → Address = 公钥哈希 + 校验 + 编码
/////////////////////////////////////////////////////////////////

func (w *Wallet) Address() []byte {

	// Step 1: 公钥 Hash160
	pubHash := PublicKeyHash(w.PublicKey)

	// Step 2: 加版本号
	versionedHash := append([]byte{Version}, pubHash...)

	// Step 3: 计算校验和
	checksum := Checksum(versionedHash)

	// Step 4: 拼接完整 payload
	fullHash := append(versionedHash, checksum...)

	// Step 5: Base58 编码（可读地址）
	address := Base58Encode(fullHash)

	/////////////////////////////////////////////////////////////////
	// 🧪 Debug 输出（开发阶段用）
	/////////////////////////////////////////////////////////////////
	// fmt.Printf("pub key: %x\n", w.PublicKey)
	// fmt.Printf("pub hash: %x\n", pubHash)
	fmt.Printf("address: %s\n", address)

	return address
}

/////////////////////////////////////////////////////////////////
// 🔐 公钥 Hash（Hash160）
//
// 标准流程：
//
// SHA256(pubKey)
//     ↓
// RIPEMD160
//     ↓
// pubKeyHash（20 bytes）
/////////////////////////////////////////////////////////////////

func PublicKeyHash(pubKey []byte) []byte {

	// Step 1: SHA256
	pubHash := sha256.Sum256(pubKey)

	// Step 2: RIPEMD160（缩短长度）
	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	utils.Handle(err)

	return hasher.Sum(nil)
}

/////////////////////////////////////////////////////////////////
// 🧾 校验和（防错误输入）
//
// checksum = SHA256(SHA256(payload))[:4]
/////////////////////////////////////////////////////////////////

func Checksum(payload []byte) []byte {
	first := sha256.Sum256(payload)
	second := sha256.Sum256(first[:])

	return second[:ChecksumLength]
}

/////////////////////////////////////////////////////////////////
// 🔄 私钥反序列化
//
// 用途：
// - 从文件恢复钱包
// - 签名交易
/////////////////////////////////////////////////////////////////

func BytesToPrivateKey(data []byte) *ecdsa.PrivateKey {
	key, err := x509.ParseECPrivateKey(data)
	utils.Handle(err)
	return key
}
