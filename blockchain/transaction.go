package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

// Transaction 代表区块链上的一笔交易
// ID      : 交易的唯一标识，由交易内容哈希而来
// Inputs  : 该交易消耗的 UTXO 引用列表（资金来源）
// Outputs : 该交易产生的新 UTXO 列表（资金去向）
type Transaction struct {
	ID      []byte
	Inputs  []TXInput
	Outputs []TXOutput
}

// TXOutput 代表一笔交易的输出，即一个未花费的 UTXO
// Value  : 该输出锁定的代币数量
// PubKey : 能够解锁该输出的公钥（此处简化为地址字符串）
type TXOutput struct {
	Value  int
	PubKey string
}

// TXInput 代表一笔交易的输入，指向某个之前交易的输出（UTXO）
// ID  : 被引用的前序交易 ID
// Out : 被引用输出在前序交易 Outputs 切片中的索引
// Sig : 解锁脚本（此处简化为发送方地址，用于证明有权花费该 UTXO）
type TXInput struct {
	ID  []byte
	Out int
	Sig string
}

// setID 将交易序列化后取 SHA-256 哈希，作为该交易的唯一 ID
// 保证交易内容一旦改变，ID 随之改变，防止篡改
func (tx *Transaction) setID() {
	var encoded bytes.Buffer
	encoder := gob.NewEncoder(&encoded)
	err := encoder.Encode(tx)
	Handle(err)

	hash := sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

// CoinBaseTx 创建一笔 Coinbase 交易（挖矿奖励交易）
// Coinbase 交易没有真实输入，凭空产生奖励，用于激励矿工
// to   : 奖励的接收方地址
// data : 矿工自定义数据（类似比特币的 coinbase 字段）；若为空则自动生成
func CoinBaseTx(to string, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Reward to '%s'", to)
	}

	// Coinbase 输入的特征：引用交易 ID 为空，输出索引为 -1
	txIn := TXInput{[]byte{}, -1, data}
	// 固定奖励 100 个代币
	txOut := TXOutput{100, to}

	tx := Transaction{nil, []TXInput{txIn}, []TXOutput{txOut}}
	tx.setID()

	return &tx
}

// NewTransaction 创建一笔普通转账交易（UTXO 模型）
// from   : 发送方地址
// to     : 接收方地址
// amount : 转账金额
// chain  : 当前区块链，用于查询可用 UTXO
func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {
	var inputs []TXInput
	var outputs []TXOutput

	// 从链上找到 from 地址足够支付 amount 的 UTXO 集合
	// acc         : 找到的 UTXO 总金额
	// validOutputs: 可用的 UTXO，格式为 map[txID][]outputIndex
	acc, validOutputs := chain.FindSpendableOutputs(from, amount)

	if acc < amount {
		log.Panic("ERROR: Not enough funds")
	}

	// 将所有选中的 UTXO 转换为交易输入
	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			input := TXInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	// 创建转账输出：将 amount 锁定给接收方
	outputs = append(outputs, TXOutput{amount, to})

	// 若收集到的 UTXO 总额超过转账金额，将差额作为找零返还给发送方
	if acc > amount {
		outputs = append(outputs, TXOutput{acc - amount, from})
	}

	tx := Transaction{nil, inputs, outputs}
	tx.setID()

	return &tx
}

// IsCoinBase 判断当前交易是否为 Coinbase 交易
// Coinbase 交易的特征：只有一个输入，且该输入的 ID 为空、索引为 -1
func (tx *Transaction) IsCoinBase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// CanUnlock 判断该输入是否由 data（地址）持有者创建
// 即验证发送方是否有权花费这笔 UTXO（简化版签名验证）
func (in *TXInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// CanBeUnlocked 判断该输出是否可被 data（地址）持有者解锁花费
// 即验证接收方地址是否匹配（简化版锁定脚本验证）
func (out *TXOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
