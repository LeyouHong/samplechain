package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/LeyouHong/samplechain/utils"
)

// 交易类型常量
const (
	TxTypeTransfer = 0 // 普通转账
	TxTypeDeploy   = 1 // 部署合约
	TxTypeCall     = 2 // 调用合约
)

// Transaction 表示区块链上的一笔交易（账户模型）
// 不再有 UTXO 的 Inputs/Outputs，改为直接记录发送方、接收方和金额
type Transaction struct {
	ID   []byte // 交易哈希，由交易内容计算得出
	Type int    // 交易类型：Transfer / Deploy / Call

	From     []byte // 发送方地址（Base58 字节），coinbase 交易时为 nil
	To       []byte // 接收方地址，部署合约时为 nil（地址由链生成）
	Value    int64  // 转账金额，单位为链的最小货币单位
	Data     []byte // 附加数据：Deploy 时为合约字节码，Call 时为 ABI 编码调用数据，Transfer 时为空
	GasLimit uint64 // 愿意消耗的最大 Gas 量
	Nonce    uint64 // 发送方的交易计数，每发一笔交易 +1，防止重放攻击

	Signature []byte // ECDSA 签名，由 (r, s) 拼接而成，coinbase 交易为 nil
}

// ─── 哈希与序列化 ────────────────────────────────────────────

// Hash 计算交易的 SHA256 哈希，作为交易 ID
// 计算时排除 ID 和 Signature 字段，避免循环依赖
func (tx *Transaction) Hash() []byte {
	txCopy := *tx
	txCopy.ID = []byte{}
	txCopy.Signature = nil

	hash := sha256.Sum256(txCopy.Serialize())
	return hash[:]
}

// Serialize 将交易使用 gob 编码序列化为字节切片
// 用于持久化存储和网络传输
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
func DeserializeTransaction(data []byte) Transaction {
	var transaction Transaction

	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&transaction)
	utils.Handle(err)
	return transaction
}

// ─── 构造交易 ────────────────────────────────────────────────

// CoinBaseTx 创建一笔 coinbase 交易（挖矿奖励）
// coinbase 交易没有发送方，由链直接向矿工发放固定奖励
// to：矿工地址（Base58 字节）；data：附加说明，可为空
func CoinBaseTx(to []byte, data string) *Transaction {
	tx := &Transaction{
		Type:     TxTypeTransfer,
		From:     nil, // coinbase 无发送方
		To:       to,
		Value:    20, // 固定挖矿奖励 20 个单位
		Data:     []byte(data),
		GasLimit: 0,
		Nonce:    0,
	}
	tx.ID = tx.Hash()
	return tx
}

// NewTransaction 构造一笔普通转账交易
// from/to：发送方和接收方地址（Base58 字节）
// value：转账金额；privKey：发送方私钥用于签名；stateDB：用于读取 nonce 和校验余额
func NewTransaction(from, to []byte, value int64, privKey ecdsa.PrivateKey, stateDB *StateDB) *Transaction {
	// 检查发送方余额是否足够
	if stateDB.GetBalance(from) < value {
		log.Panic("Error: not enough funds")
	}

	tx := &Transaction{
		Type:     TxTypeTransfer,
		From:     from,
		To:       to,
		Value:    value,
		GasLimit: 21000,                  // 普通转账固定消耗 21000 Gas
		Nonce:    stateDB.GetNonce(from), // 从状态层读取当前 nonce，防重放
	}
	tx.ID = tx.Hash()
	tx.Sign(privKey)

	return tx
}

// NewDeployTx 构造一笔合约部署交易
// from：部署方地址；bytecode：编译后的合约字节码；gasLimit：愿意消耗的最大 Gas
func NewDeployTx(from []byte, bytecode []byte, gasLimit uint64, privKey ecdsa.PrivateKey, stateDB *StateDB) *Transaction {
	tx := &Transaction{
		Type:     TxTypeDeploy,
		From:     from,
		To:       nil, // 部署时接收方为空，合约地址由链派生
		Value:    0,
		Data:     bytecode,
		GasLimit: gasLimit,
		Nonce:    stateDB.GetNonce(from),
	}
	tx.ID = tx.Hash()
	tx.Sign(privKey)

	return tx
}

// NewCallTx 构造一笔合约调用交易
// from：调用方地址；to：合约地址；callData：ABI 编码的函数调用数据；value：附带的原生币数量
func NewCallTx(from, to []byte, callData []byte, value int64, gasLimit uint64, privKey ecdsa.PrivateKey, stateDB *StateDB) *Transaction {
	tx := &Transaction{
		Type:     TxTypeCall,
		From:     from,
		To:       to,
		Value:    value,
		Data:     callData,
		GasLimit: gasLimit,
		Nonce:    stateDB.GetNonce(from),
	}
	tx.ID = tx.Hash()
	tx.Sign(privKey)

	return tx
}

// ─── 签名与验证 ──────────────────────────────────────────────

// Sign 使用 ECDSA 私钥对整笔交易签名
// 与 UTXO 模型不同，账户模型只需对整笔交易签名一次，不需要逐输入签名
func (tx *Transaction) Sign(privKey ecdsa.PrivateKey) {
	if tx.IsCoinBase() {
		return
	}

	dataToSign := fmt.Sprintf("%x", tx.Hash())
	r, s, err := ecdsa.Sign(rand.Reader, &privKey, []byte(dataToSign))
	if err != nil {
		log.Panic(err)
	}
	// 将 r、s 拼接为签名字节切片（各占一半）
	tx.Signature = append(r.Bytes(), s.Bytes()...)
}

// Verify 验证交易签名是否合法
// pubKey：发送方公钥的原始字节（x、y 坐标各占一半拼接而成）
func (tx *Transaction) Verify(pubKey []byte) bool {
	if tx.IsCoinBase() {
		return true
	}

	dataToVerify := fmt.Sprintf("%x", tx.Hash())

	// 从签名字节切片中还原 r、s 分量
	r, s := big.Int{}, big.Int{}
	sigLen := len(tx.Signature)
	r.SetBytes(tx.Signature[:(sigLen / 2)])
	s.SetBytes(tx.Signature[(sigLen / 2):])

	// 从公钥字节切片中还原 x、y 坐标
	x, y := big.Int{}, big.Int{}
	keyLen := len(pubKey)
	x.SetBytes(pubKey[:(keyLen / 2)])
	y.SetBytes(pubKey[(keyLen / 2):])

	curve := elliptic.P256()
	rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}

	return ecdsa.Verify(&rawPubKey, []byte(dataToVerify), &r, &s)
}

// ─── 辅助方法 ────────────────────────────────────────────────

// IsCoinBase 判断是否为 coinbase 交易
// 判断依据：发送方为 nil 且类型为普通转账
func (tx *Transaction) IsCoinBase() bool {
	return tx.From == nil && tx.Type == TxTypeTransfer
}

// String 返回交易的可读字符串，用于调试和日志输出
func (tx Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))
	lines = append(lines, fmt.Sprintf("    Type:      %d", tx.Type))
	lines = append(lines, fmt.Sprintf("    From:      %x", tx.From))
	lines = append(lines, fmt.Sprintf("    To:        %x", tx.To))
	lines = append(lines, fmt.Sprintf("    Value:     %d", tx.Value))
	lines = append(lines, fmt.Sprintf("    Nonce:     %d", tx.Nonce))
	lines = append(lines, fmt.Sprintf("    GasLimit:  %d", tx.GasLimit))
	lines = append(lines, fmt.Sprintf("    Signature: %x", tx.Signature))

	if len(tx.Data) > 0 {
		lines = append(lines, fmt.Sprintf("    Data:      %x", tx.Data))
	}

	return strings.Join(lines, "\n")
}
