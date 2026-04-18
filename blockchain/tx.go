package blockchain

import (
	"bytes"

	"github.com/LeyouHong/samplechain/wallet"
)

/////////////////////////////////////////////////////////////////
// 🧠 核心背景（一定要理解）
//
// 在 UTXO 模型中：
// ❌ 没有“账户余额”
// ✅ 钱 = 一堆未花费的输出（UTXO）
//
// 一笔交易：
// Input  → 消耗旧的 UTXO（花钱）
// Output → 生成新的 UTXO（收钱）
/////////////////////////////////////////////////////////////////

// TXOutput 表示一笔“未花费输出”（UTXO）
// 👉 可以理解为：一笔“钱”
//
// 一旦创建：
// - 不可修改
// - 只能被后续交易作为 Input 消费
type TXOutput struct {
	Value int // 💰 金额（这笔钱有多少币）

	// 🔒 锁（Lock）
	// 只有拥有对应私钥的人才能花这笔钱
	// 实际存的是：公钥的 hash（不是地址字符串）
	PubKeyHash []byte
}

/////////////////////////////////////////////////////////////////
// 🧾 TXInput（花钱的动作）
//
// TXInput 并不是钱，而是：
// 👉 “引用旧的钱 + 提供解锁证明”
/////////////////////////////////////////////////////////////////

type TXInput struct {
	ID []byte // 🧾 被引用的“之前交易”的 ID

	// 📍 指向该交易里的第几个 output
	// （因为一笔交易可能有多个 output）
	Out int

	// ✍️ 签名（Signature）
	// 证明：你确实拥有这笔钱对应的私钥
	Signature []byte

	// 🔑 公钥（Public Key）
	// 用于验证 Signature 是否正确
	PubKey []byte
}

/////////////////////////////////////////////////////////////////
// 🏭 创建一笔新的 Output（生成钱）
//
// value   → 金额
// address → 收款地址
/////////////////////////////////////////////////////////////////

func NewTXOutput(value int, address string) *TXOutput {
	txo := &TXOutput{value, nil}

	// 🔒 给这笔钱上锁
	// 地址 → pubKeyHash
	txo.Lock([]byte(address))

	return txo
}

/////////////////////////////////////////////////////////////////
// 🔓 判断这个 Input 是否“属于某个用户”
//
// 本质：
// PubKey → Hash → 是否等于目标 pubKeyHash
/////////////////////////////////////////////////////////////////

func (in *TXInput) UsesKey(pubKeyHash []byte) bool {
	// 用输入中的公钥计算 hash
	lockingHash := wallet.PublicKeyHash(in.PubKey)

	// 比较是否匹配
	return bytes.Equal(lockingHash, pubKeyHash)
}

/////////////////////////////////////////////////////////////////
// 🔒 给 Output 上锁（核心逻辑）
//
// address → Base58Decode → pubKeyHash
/////////////////////////////////////////////////////////////////

func (out *TXOutput) Lock(address []byte) {
	// Step 1: Base58 解码地址
	decoded := wallet.Base58Decode(address)

	// Step 2: 去掉 version + checksum
	// address = version + pubKeyHash + checksum
	pubKeyHash := decoded[1 : len(decoded)-wallet.ChecksumLength]

	// Step 3: 保存真正的锁
	out.PubKeyHash = pubKeyHash
}

/////////////////////////////////////////////////////////////////
// 🔐 判断这笔钱是不是“锁给某个人”
//
// 常用于：
// - 找 UTXO
// - 计算余额
/////////////////////////////////////////////////////////////////

func (out *TXOutput) IsLockedWithKey(pubKeyHash []byte) bool {
	return bytes.Equal(out.PubKeyHash, pubKeyHash)
}

/////////////////////////////////////////////////////////////////
// 🧠 整体流程总结（非常重要）
//
// 💸 转账过程：
//
// 1️⃣ 找钱（UTXO）
//   找到属于你的 TXOutput
//
// 2️⃣ 构造 Input
//   引用这些 output
//   + 放入 PubKey
//   + 后续签名
//
// 3️⃣ 构造 Output
//   给对方 amount
//   给自己找零
//
// 4️⃣ 验证（节点做的事）
//   PubKey → hash → 是否匹配 Output.PubKeyHash
//
// ✔️ 成立 → 可以花钱
// ❌ 不成立 → 非法交易
/////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////
// 🚀 你这个实现 vs Bitcoin
//
// 你现在：
//   PubKeyHash + Signature（简化版）
//
// Bitcoin：
//   scriptPubKey（锁定脚本）
//   scriptSig（解锁脚本）
//
// 👉 更强：支持多签、时间锁、复杂条件
/////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////
// 🧠 一句话总结（面试可以直接说）
//
// TXOutput = 钱 + 锁
// TXInput  = 引用钱 + 解锁证明
/////////////////////////////////////////////////////////////////
