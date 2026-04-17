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

type Wallet struct {
	PrivateKey []byte // ⚠️ 改成 byte
	PublicKey  []byte
}

const (
	checksumLength = 4
	version        = byte(0x00)
)

// 创建钱包
func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	return &Wallet{private, public}
}

// 生成 key pair（关键改动）
func NewKeyPair() ([]byte, []byte) {
	curve := elliptic.P256()

	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	utils.Handle(err)

	// ✅ 转成 bytes（解决 gob 问题）
	privBytes, err := x509.MarshalECPrivateKey(private)
	utils.Handle(err)

	pub := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)

	return privBytes, pub
}

// 地址生成
func (w *Wallet) Address() []byte {
	pubHash := PublicKeyHash(w.PublicKey)

	versionedHash := append([]byte{version}, pubHash...)
	checksum := Checksum(versionedHash)
	fullHash := append(versionedHash, checksum...)

	address := Base58Encode(fullHash)

	fmt.Printf("pub key: %x\n", w.PublicKey)
	fmt.Printf("pub hash: %x\n", pubHash)
	fmt.Printf("address: %s\n", address)

	return address
}

// 公钥 hash
func PublicKeyHash(pubKey []byte) []byte {
	pubHash := sha256.Sum256(pubKey)

	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	utils.Handle(err)

	return hasher.Sum(nil)
}

// 校验和
func Checksum(payload []byte) []byte {
	firstHash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(firstHash[:])

	return secondHash[:checksumLength]
}

// 如果你后面要用私钥
func BytesToPrivateKey(data []byte) *ecdsa.PrivateKey {
	key, err := x509.ParseECPrivateKey(data)
	utils.Handle(err)
	return key
}
