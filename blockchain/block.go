// 本文件定义区块结构及相关操作，区块有效性通过工作量证明（PoW）机制保证
package blockchain

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/LeyouHong/samplechain/utils"
)

// Block 表示区块链中的一个区块
type Block struct {
	Timestamp    int64          // 区块创建时间（Unix 时间戳，秒）
	Hash         []byte         // 当前区块的哈希值，由 PoW 计算得出
	Transactions []*Transaction // 本区块打包的交易列表
	PrevHash     []byte         // 前一个区块的哈希值，用于链式连接，保证不可篡改
	Nonce        int            // PoW 挖矿时找到的随机数，验证时可重现目标哈希
	Height       int            // 区块高度（从 0 开始），即该区块在链中的位置序号
}

// HashTransactions 计算区块内所有交易的 Merkle 树根哈希
// 将每笔交易序列化后构建 Merkle 树，根节点哈希代表本区块的交易摘要
// 任意一笔交易被篡改都会导致根哈希改变，从而使区块哈希失效
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte

	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.Serialize())
	}
	tree := NewMerkleTree(txHashes)

	return tree.RootNode.Data
}

// CreateBlock 创建一个新区块并通过 PoW 挖矿计算其哈希
// txs：本区块打包的交易列表；prevHash：前一区块的哈希；height：当前区块高度
// 挖矿完成后将 Nonce 和 Hash 写回区块，返回合法区块
func CreateBlock(txs []*Transaction, prevHash []byte, height int) *Block {
	block := &Block{time.Now().Unix(), []byte{}, txs, prevHash, 0, height}

	// 创建 PoW 实例并执行挖矿，找到满足难度目标的 nonce 和对应哈希
	pow := NewProofOfWork(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Genesis 创建创世区块（链中第一个区块）
// 创世区块没有前驱，PrevHash 为空，高度为 0
// coinbase：创世区块中唯一的一笔交易，用于向初始矿工发放奖励
func Genesis(coinbase *Transaction) *Block {
	return CreateBlock([]*Transaction{coinbase}, []byte{}, 0)
}

// Serialize 将区块使用 gob 编码序列化为字节切片
// 用于将区块持久化到数据库或通过网络发送给其他节点
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)

	utils.Handle(err)

	return res.Bytes()
}

// Deserialize 将字节切片反序列化为 Block 结构体
// 用于从数据库读取或从网络接收区块数据后还原区块
func Deserialize(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	err := decoder.Decode(&block)

	utils.Handle(err)

	return &block
}
