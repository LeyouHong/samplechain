package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
)

// Account 表示区块链上的一个账户
// 普通账户：CodeHash 为 nil，StorageRoot 为 nil
// 合约账户：CodeHash 指向合约字节码，StorageRoot 指向合约存储的 Merkle 根
type Account struct {
	Address     []byte // 账户地址（Base58 字节）
	Balance     int64  // 账户余额
	Nonce       uint64 // 交易计数，每发出一笔交易 +1，防止重放攻击
	PublicKey   []byte // 账户公钥原始字节（x、y 坐标各占一半拼接），注册账户时写入，验签时使用
	CodeHash    []byte // 合约字节码的 SHA256 哈希，普通账户为 nil
	StorageRoot []byte // 合约存储树的根哈希，普通账户为 nil
}

// IsContract 判断当前账户是否为合约账户
func (a *Account) IsContract() bool {
	return len(a.CodeHash) > 0
}

// Serialize 将账户序列化为字节切片，用于存入 BadgerDB
func (a *Account) Serialize() []byte {
	var encoded bytes.Buffer
	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(a)
	if err != nil {
		log.Panic(err)
	}
	return encoded.Bytes()
}

// DeserializeAccount 将字节切片反序列化为 Account 结构体
func DeserializeAccount(data []byte) *Account {
	var account Account
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&account)
	if err != nil {
		log.Panic(err)
	}
	return &account
}
