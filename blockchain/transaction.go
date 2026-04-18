package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/LeyouHong/samplechain/utils"
	"github.com/LeyouHong/samplechain/wallet"
)

/////////////////////////////////////////////////////////////////
// 🧠 核心背景（必须理解）
//
// Transaction = 状态转移
//
// 在 UTXO 模型中：
// 一笔交易 = 消耗旧的 UTXO + 生成新的 UTXO
//
// Inputs  → 花谁的钱
// Outputs → 钱给谁
/////////////////////////////////////////////////////////////////

// Transaction 表示一笔交易
type Transaction struct {
	ID      []byte     // 🧾 交易唯一 ID（hash）
	Inputs  []TXInput  // 💸 输入（引用旧的 UTXO）
	Outputs []TXOutput // 💰 输出（生成新的 UTXO）
}

/////////////////////////////////////////////////////////////////
// 🧱 序列化（用于 hash / 存储）
/////////////////////////////////////////////////////////////////

func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	encoder := gob.NewEncoder(&encoded)
	err := encoder.Encode(tx)
	utils.Handle(err)

	return encoded.Bytes()
}

/////////////////////////////////////////////////////////////////
// 🔑 计算交易 hash（ID）
//
// ⚠️ 注意：必须先清空 ID 再 hash（否则递归）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{} // ❗ 避免 hash 包含自身 ID

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

/////////////////////////////////////////////////////////////////
// 🧾 设置交易 ID（本质就是 hash）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) setID() {
	var encoded bytes.Buffer

	encoder := gob.NewEncoder(&encoded)
	err := encoder.Encode(tx)
	utils.Handle(err)

	hash := sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

/////////////////////////////////////////////////////////////////
// ⛏️ Coinbase 交易（挖矿奖励）
//
// 特点：
// - 没有真实输入
// - 凭空产生币
/////////////////////////////////////////////////////////////////

func CoinBaseTx(to string, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Reward to '%s'", to)
	}

	// Coinbase 特殊 input
	txIn := TXInput{
		ID:        []byte{},
		Out:       -1,
		Signature: nil,
		PubKey:    []byte(data),
	}

	// 奖励 100
	txOut := NewTXOutput(100, to)

	tx := Transaction{
		nil,
		[]TXInput{txIn},
		[]TXOutput{*txOut},
	}

	tx.setID()

	return &tx
}

/////////////////////////////////////////////////////////////////
// 💸 创建普通交易（核心逻辑）
//
// 流程：
// 1️⃣ 找 UTXO
// 2️⃣ 构造 inputs
// 3️⃣ 构造 outputs
// 4️⃣ 签名
/////////////////////////////////////////////////////////////////

func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {

	var inputs []TXInput
	var outputs []TXOutput

	// 1️⃣ 加载钱包
	wallets, err := wallet.CreateWallets()
	utils.Handle(err)

	w := wallets.GetWallet(from)

	// 计算发送者 pubKeyHash
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	// 2️⃣ 找可用 UTXO
	acc, validOutputs := chain.FindSpendableOutputs(pubKeyHash, amount)

	if acc < amount {
		log.Panic("ERROR: Not enough funds")
	}

	// 3️⃣ 构造 Inputs（引用旧钱）
	for txid, outs := range validOutputs {

		txID, err := hex.DecodeString(txid)
		utils.Handle(err)

		for _, out := range outs {
			input := TXInput{
				ID:     txID,
				Out:    out,
				PubKey: w.PublicKey, // 🔑 解锁用
			}
			inputs = append(inputs, input)
		}
	}

	// 4️⃣ 构造 Outputs（新钱）
	outputs = append(outputs, *NewTXOutput(amount, to))

	// 找零
	if acc > amount {
		outputs = append(outputs, *NewTXOutput(acc-amount, from))
	}

	tx := Transaction{nil, inputs, outputs}

	tx.ID = tx.Hash()

	// 5️⃣ 签名
	privKey := wallet.BytesToPrivateKey(w.PrivateKey)
	chain.SignTransaction(&tx, *privKey)

	return &tx
}

/////////////////////////////////////////////////////////////////
// 🔍 判断是否是 coinbase
/////////////////////////////////////////////////////////////////

func (tx *Transaction) IsCoinBase() bool {
	return len(tx.Inputs) == 1 &&
		len(tx.Inputs[0].ID) == 0 &&
		tx.Inputs[0].Out == -1
}

/////////////////////////////////////////////////////////////////
// ✍️ 签名交易（核心安全逻辑）
//
// 思路：
// 每个 input 都要签名
/////////////////////////////////////////////////////////////////

func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {

	if tx.IsCoinBase() {
		return
	}

	// 确保引用的交易存在
	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction not found")
		}
	}

	txCopy := tx.TrimmedCopy()

	for inIdx, in := range txCopy.Inputs {

		prevTX := prevTXs[hex.EncodeToString(in.ID)]

		// 用锁定脚本替换 pubkey
		txCopy.Inputs[inIdx].Signature = nil
		txCopy.Inputs[inIdx].PubKey = prevTX.Outputs[in.Out].PubKeyHash

		txCopy.ID = txCopy.Hash()

		txCopy.Inputs[inIdx].PubKey = nil

		// 🔑 用私钥签名
		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		utils.Handle(err)

		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inIdx].Signature = signature
	}
}

/////////////////////////////////////////////////////////////////
// ✂️ 生成签名用的副本（去掉签名）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TXInput
	var outputs []TXOutput

	for _, in := range tx.Inputs {
		inputs = append(inputs, TXInput{
			in.ID,
			in.Out,
			nil, // ❗ 去掉签名
			nil,
		})
	}

	for _, out := range tx.Outputs {
		outputs = append(outputs, TXOutput{
			out.Value,
			out.PubKeyHash,
		})
	}

	return Transaction{tx.ID, inputs, outputs}
}

/////////////////////////////////////////////////////////////////
// 🔍 验证交易签名（共识核心）
//
// 节点会执行这个函数！
/////////////////////////////////////////////////////////////////

func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {

	if tx.IsCoinBase() {
		return true
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction not found")
		}
	}

	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256()

	for inIdx, in := range tx.Inputs {

		prevTX := prevTXs[hex.EncodeToString(in.ID)]

		txCopy.Inputs[inIdx].Signature = nil
		txCopy.Inputs[inIdx].PubKey = prevTX.Outputs[in.Out].PubKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inIdx].PubKey = nil

		// 拆 signature
		r := big.Int{}
		s := big.Int{}
		sigLen := len(in.Signature)

		r.SetBytes(in.Signature[:sigLen/2])
		s.SetBytes(in.Signature[sigLen/2:])

		// 拆 pubkey
		x := big.Int{}
		y := big.Int{}
		keyLen := len(in.PubKey)

		x.SetBytes(in.PubKey[:keyLen/2])
		y.SetBytes(in.PubKey[keyLen/2:])

		rawPubKey := ecdsa.PublicKey{curve, &x, &y}

		if !ecdsa.Verify(&rawPubKey, txCopy.ID, &r, &s) {
			return false
		}
	}

	return true
}

/////////////////////////////////////////////////////////////////
// 🖨️ 打印交易（调试用）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))

	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("     Input %d:", i))
		lines = append(lines, fmt.Sprintf("       TXID: %x", input.ID))
		lines = append(lines, fmt.Sprintf("       Out: %d", input.Out))
		lines = append(lines, fmt.Sprintf("       Signature: %x", input.Signature))
		lines = append(lines, fmt.Sprintf("       PubKey: %x", input.PubKey))
	}

	for i, output := range tx.Outputs {
		lines = append(lines, fmt.Sprintf("     Output %d:", i))
		lines = append(lines, fmt.Sprintf("       Value: %d", output.Value))
		lines = append(lines, fmt.Sprintf("       PubKeyHash: %x", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}
