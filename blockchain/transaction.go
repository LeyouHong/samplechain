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
// 🧠 区块链核心概念
//
// Transaction（交易）= 状态转移
//
// UTXO 模型：
// - Input  = 花掉以前的钱（引用旧 UTXO）
// - Output = 生成新的钱（新的 UTXO）
/////////////////////////////////////////////////////////////////

// Transaction 一笔交易结构
type Transaction struct {
	ID      []byte     // 🔑 交易 hash（唯一 ID）
	Inputs  []TXInput  // 💸 花的钱（引用 UTXO）
	Outputs []TXOutput // 💰 新的钱（生成 UTXO）
}

/////////////////////////////////////////////////////////////////
// 🧾 序列化（用于 hash / 存储）
/////////////////////////////////////////////////////////////////

func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	encoder := gob.NewEncoder(&encoded)
	err := encoder.Encode(tx)
	utils.Handle(err)

	return encoded.Bytes()
}

/////////////////////////////////////////////////////////////////
// 🔑 计算交易 Hash（交易 ID）
//
// ⚠️ 必须先清空 ID，否则 hash 会自引用
/////////////////////////////////////////////////////////////////

func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{} // ❗ 防止递归 hash

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

/////////////////////////////////////////////////////////////////
// 🧾 设置交易 ID（本质就是 hash）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) setID() {
	var encoded bytes.Buffer
	var hash [32]byte

	encoder := gob.NewEncoder(&encoded)
	err := encoder.Encode(tx)
	utils.Handle(err)

	hash = sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

/////////////////////////////////////////////////////////////////
// ⛏️ Coinbase 交易（挖矿奖励）
//
// 特点：
// - 没有 input
// - 系统发钱
/////////////////////////////////////////////////////////////////

func CoinBaseTx(to string, data string) *Transaction {

	if data == "" {
		randomData := make([]byte, 24)
		_, err := rand.Read(randomData)
		utils.Handle(err)

		data = fmt.Sprintf("%x", randomData)
	}

	// 特殊 input（没有真实来源）
	txIn := TXInput{
		ID:        []byte{},
		Out:       -1,
		Signature: nil,
		PubKey:    []byte(data),
	}

	// 固定奖励
	txOut := NewTXOutput(20, to)

	tx := Transaction{
		nil,
		[]TXInput{txIn},
		[]TXOutput{*txOut},
	}

	tx.ID = tx.Hash() // 生成交易 ID

	tx.setID()

	return &tx
}

/////////////////////////////////////////////////////////////////
// 💸 普通转账交易（核心逻辑）
/////////////////////////////////////////////////////////////////

func NewTransaction(from, to string, amount int, UTXO *UTXOSet) *Transaction {

	var inputs []TXInput
	var outputs []TXOutput

	// 1️⃣ 加载钱包
	wallets, err := wallet.CreateWallets()
	utils.Handle(err)

	w := wallets.GetWallet(from)

	// sender pubKeyHash（用于找 UTXO）
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	// 2️⃣ 查 UTXO（找够钱）
	acc, validOutputs := UTXO.FindSpendableOutputs(pubKeyHash, amount)

	if acc < amount {
		log.Panic("ERROR: Not enough funds")
	}

	// 3️⃣ 构造 Inputs（花掉旧 UTXO）
	for txid, outs := range validOutputs {

		txID, err := hex.DecodeString(txid)
		utils.Handle(err)

		for _, out := range outs {
			input := TXInput{
				ID:     txID,
				Out:    out,
				PubKey: w.PublicKey, // 🔑 用于验证 ownership
			}
			inputs = append(inputs, input)
		}
	}

	// 4️⃣ 构造 Outputs（生成新 UTXO）
	outputs = append(outputs, *NewTXOutput(amount, to))

	// 找零
	if acc > amount {
		outputs = append(outputs, *NewTXOutput(acc-amount, from))
	}

	tx := Transaction{nil, inputs, outputs}

	// 5️⃣ 生成 tx hash
	tx.ID = tx.Hash()

	// 6️⃣ 签名交易（防止伪造）
	privKey := wallet.BytesToPrivateKey(w.PrivateKey)
	UTXO.Blockchain.SignTransaction(&tx, *privKey)

	return &tx
}

/////////////////////////////////////////////////////////////////
// 🔍 判断 coinbase
/////////////////////////////////////////////////////////////////

func (tx *Transaction) IsCoinBase() bool {
	return len(tx.Inputs) == 1 &&
		len(tx.Inputs[0].ID) == 0 &&
		tx.Inputs[0].Out == -1
}

/////////////////////////////////////////////////////////////////
// ✍️ 签名（核心安全机制）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {

	if tx.IsCoinBase() {
		return
	}

	// 校验 prev tx 是否存在
	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction not found")
		}
	}

	txCopy := tx.TrimmedCopy()

	for inIdx, in := range txCopy.Inputs {

		prevTX := prevTXs[hex.EncodeToString(in.ID)]

		// 临时替换 pubkey（用于 hash）
		txCopy.Inputs[inIdx].Signature = nil
		txCopy.Inputs[inIdx].PubKey = prevTX.Outputs[in.Out].PubKeyHash

		txCopy.ID = txCopy.Hash()

		txCopy.Inputs[inIdx].PubKey = nil

		// 🔑 ECDSA 签名
		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		utils.Handle(err)

		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inIdx].Signature = signature
	}
}

/////////////////////////////////////////////////////////////////
// ✂️ 生成签名用副本（去掉 signature）
/////////////////////////////////////////////////////////////////

func (tx *Transaction) TrimmedCopy() Transaction {

	var inputs []TXInput
	var outputs []TXOutput

	for _, in := range tx.Inputs {
		inputs = append(inputs, TXInput{
			in.ID,
			in.Out,
			nil, // ❗ 清除签名
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
// 🔍 验证交易（节点执行）
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

		pubKey := ecdsa.PublicKey{curve, &x, &y}

		if !ecdsa.Verify(&pubKey, txCopy.ID, &r, &s) {
			return false
		}
	}

	return true
}

/////////////////////////////////////////////////////////////////
// 🖨️ 调试输出
/////////////////////////////////////////////////////////////////

func (tx *Transaction) String() string {

	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))

	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("Input %d:", i))
		lines = append(lines, fmt.Sprintf("  TXID: %x", input.ID))
		lines = append(lines, fmt.Sprintf("  Out: %d", input.Out))
		lines = append(lines, fmt.Sprintf("  Signature: %x", input.Signature))
		lines = append(lines, fmt.Sprintf("  PubKey: %x", input.PubKey))
	}

	for i, output := range tx.Outputs {
		lines = append(lines, fmt.Sprintf("Output %d:", i))
		lines = append(lines, fmt.Sprintf("  Value: %d", output.Value))
		lines = append(lines, fmt.Sprintf("  PubKeyHash: %x", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}
