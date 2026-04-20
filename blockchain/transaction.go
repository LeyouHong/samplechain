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

// Transaction 表示区块链上的一笔交易
// 每笔交易由若干输入（TXInput）和输出（TXOutput）组成，遵循 UTXO 模型
type Transaction struct {
	ID      []byte     // 交易 ID，即对交易内容进行 SHA256 哈希后的结果
	Inputs  []TXInput  // 交易输入列表，每个输入引用某笔历史交易的输出（UTXO）
	Outputs []TXOutput // 交易输出列表，定义本次交易的接收方和金额
}

// Hash 计算并返回交易的哈希值，用作交易 ID
// 计算前将 ID 字段清空，避免循环依赖
func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{} // 清空 ID 字段，再对其余字段序列化后求哈希

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

// Serialize 将交易结构体使用 gob 编码序列化为字节切片
// 用于网络传输和持久化存储
func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

// DeserializeTransaction 将字节切片反序列化为 Transaction 结构体
// 用于从数据库读取或从网络接收数据后还原交易
func DeserializeTransaction(data []byte) Transaction {
	var transaction Transaction

	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&transaction)
	utils.Handle(err)
	return transaction
}

// CoinBaseTx 创建一笔 coinbase 交易（挖矿奖励交易）
// coinbase 交易没有有效的输入引用，是区块链中凭空产生币的唯一方式
// to：矿工的钱包地址；data：附加数据，为空时自动生成随机数据防止哈希碰撞
func CoinBaseTx(to, data string) *Transaction {
	if data == "" {
		// 生成 24 字节随机数据作为 coinbase 的附加字段，确保每笔 coinbase 交易的唯一性
		randData := make([]byte, 24)
		_, err := rand.Read(randData)
		utils.Handle(err)
		data = fmt.Sprintf("%x", randData)
	}

	// coinbase 输入：引用空交易 ID、输出索引为 -1，表示不消费任何 UTXO
	txin := TXInput{[]byte{}, -1, nil, []byte(data)}
	// coinbase 输出：向矿工地址发放固定奖励 20 个单位
	txout := NewTXOutput(20, to)

	tx := Transaction{nil, []TXInput{txin}, []TXOutput{*txout}}
	tx.ID = tx.Hash()

	return &tx
}

// NewTransaction 构造一笔普通转账交易
// w：发送方钱包；to：接收方地址；amount：转账金额；UTXO：当前 UTXO 集合
// 流程：查找可用 UTXO → 构造输入 → 构造输出（含找零）→ 签名
func NewTransaction(w *wallet.Wallet, to string, amount int, UTXO *UTXOSet) *Transaction {
	var inputs []TXInput
	var outputs []TXOutput

	// 根据发送方公钥哈希查找足够金额的可用 UTXO
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)
	acc, validOutputs := UTXO.FindSpendableOutputs(pubKeyHash, amount)

	if acc < amount {
		log.Panic("Error: not enough funds")
	}

	// 将所有可用 UTXO 转换为交易输入
	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		utils.Handle(err)

		for _, out := range outs {
			// 输入引用历史 UTXO，Signature 和 PubKey 在签名阶段填充
			input := TXInput{txID, out, nil, w.PublicKey}
			inputs = append(inputs, input)
		}
	}

	from := fmt.Sprintf("%s", w.Address())

	// 构造转账输出：向接收方转入指定金额
	outputs = append(outputs, *NewTXOutput(amount, to))

	// 若 UTXO 总额超出转账金额，构造找零输出返还给发送方
	if acc > amount {
		outputs = append(outputs, *NewTXOutput(acc-amount, from))
	}

	tx := Transaction{nil, inputs, outputs}
	tx.ID = tx.Hash()

	// 使用发送方私钥对交易进行签名，防止伪造
	privKey := wallet.BytesToPrivateKey(w.PrivateKey)
	UTXO.Blockchain.SignTransaction(&tx, *privKey)

	return &tx
}

// IsCoinBase 判断当前交易是否为 coinbase 交易
// 判断依据：只有一个输入，且该输入的交易 ID 为空，输出索引为 -1
func (tx *Transaction) IsCoinBase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// Sign 使用 ECDSA 私钥对交易的每个输入进行签名
// prevTXs：输入所引用的历史交易映射（key 为交易 ID 的十六进制字符串）
// 签名过程：对每个输入，将其 PubKey 临时替换为被引用输出的 PubKeyHash，序列化后签名
func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {
	// coinbase 交易无需签名
	if tx.IsCoinBase() {
		return
	}

	// 校验所有输入引用的历史交易是否合法
	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction is not correct")
		}
	}

	// 使用裁剪副本签名（Signature 和 PubKey 字段清空），避免循环依赖
	txCopy := tx.TrimmedCopy()

	for inId, in := range txCopy.Inputs {
		prevTX := prevTXs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil
		// 临时将 PubKey 设为被引用输出的锁定脚本（PubKeyHash），作为签名数据的一部分
		txCopy.Inputs[inId].PubKey = prevTX.Outputs[in.Out].PubKeyHash

		// 将副本格式化为字符串作为待签名数据
		dataToSign := fmt.Sprintf("%x\n", txCopy)

		// 使用 ECDSA 签名，得到 (r, s) 并拼接存入对应输入的 Signature 字段
		r, s, err := ecdsa.Sign(rand.Reader, &privKey, []byte(dataToSign))
		utils.Handle(err)
		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inId].Signature = signature
		txCopy.Inputs[inId].PubKey = nil // 签名完成后清空，准备处理下一个输入
	}
}

// Verify 验证交易中每个输入的 ECDSA 签名是否合法
// prevTXs：输入引用的历史交易映射
// 返回 true 表示所有输入的签名均通过验证
func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {
	// coinbase 交易无需验证签名
	if tx.IsCoinBase() {
		return true
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("Previous transaction not correct")
		}
	}

	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256() // 使用与签名时相同的 P-256 椭圆曲线

	for inId, in := range tx.Inputs {
		prevTx := prevTXs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[in.Out].PubKeyHash

		// 从 Signature 字节切片中还原 ECDSA 签名的 r、s 分量（各占一半）
		r := big.Int{}
		s := big.Int{}
		sigLen := len(in.Signature)
		r.SetBytes(in.Signature[:(sigLen / 2)])
		s.SetBytes(in.Signature[(sigLen / 2):])

		// 从 PubKey 字节切片中还原 ECDSA 公钥的 x、y 坐标（各占一半）
		x := big.Int{}
		y := big.Int{}
		keyLen := len(in.PubKey)
		x.SetBytes(in.PubKey[:(keyLen / 2)])
		y.SetBytes(in.PubKey[(keyLen / 2):])

		dataToVerify := fmt.Sprintf("%x\n", txCopy)

		// 用还原的公钥验证签名
		rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}
		if ecdsa.Verify(&rawPubKey, []byte(dataToVerify), &r, &s) == false {
			return false
		}
		txCopy.Inputs[inId].PubKey = nil // 清空，准备验证下一个输入
	}

	return true
}

// TrimmedCopy 返回交易的裁剪副本：每个输入的 Signature 和 PubKey 均置为 nil
// 用于签名和验证流程，确保待签名/验证的数据中不含敏感字段
func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TXInput
	var outputs []TXOutput

	for _, in := range tx.Inputs {
		inputs = append(inputs, TXInput{in.ID, in.Out, nil, nil})
	}

	for _, out := range tx.Outputs {
		outputs = append(outputs, TXOutput{out.Value, out.PubKeyHash})
	}

	txCopy := Transaction{tx.ID, inputs, outputs}

	return txCopy
}

// String 返回交易的可读字符串表示，用于调试和日志输出
// 格式化展示交易 ID、所有输入（含 TXID、索引、签名、公钥）和所有输出（含金额、锁定脚本）
func (tx Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))
	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("     Input %d:", i))
		lines = append(lines, fmt.Sprintf("       TXID:     %x", input.ID))
		lines = append(lines, fmt.Sprintf("       Out:       %d", input.Out))
		lines = append(lines, fmt.Sprintf("       Signature: %x", input.Signature))
		lines = append(lines, fmt.Sprintf("       PubKey:    %x", input.PubKey))
	}

	for i, output := range tx.Outputs {
		lines = append(lines, fmt.Sprintf("     Output %d:", i))
		lines = append(lines, fmt.Sprintf("       Value:  %d", output.Value))
		lines = append(lines, fmt.Sprintf("       Script: %x", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}
