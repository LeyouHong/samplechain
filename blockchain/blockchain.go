package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"

	"github.com/LeyouHong/samplechain/utils"
	"github.com/dgraph-io/badger"
)

// ========================
// 数据库存储相关路径
// ========================

// dbPath  : Badger DB 数据目录（存 block + utxo）
// dbFile  : 用来判断 DB 是否存在（MANIFEST 是 Badger 标志文件）
// genesisData : 创世区块默认信息（第一笔交易）
const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis"
)

// ========================
// 区块链结构
// ========================

// BlockChain = 一条链 + 一个数据库
// LastHash   : 当前链尾 block hash（快速定位最新 block）
// Database   : Badger KV 存储（key-value = hash -> block）
type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// BlockChainIterator
// 用于从“最新 block → genesis block”遍历
type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// ========================
// 创建 / 加载链
// ========================

// InitBlockChain：第一次创建链（必须是空的）
func InitBlockChain(address string) *BlockChain {

	// 如果 DB 已存在 → 禁止重复创建
	if utils.DBexists(dbFile) {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	var lastHash []byte

	// 打开 Badger DB
	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath
	opts.Logger = nil // 关闭 Badger 日志输出

	db, err := badger.Open(opts)
	utils.Handle(err)

	// 写入创世区块
	err = db.Update(func(txn *badger.Txn) error {

		// 1. 创建 coinbase 交易（挖矿奖励）
		cbtx := CoinBaseTx(address, genesisData)

		// 2. 创建 genesis block
		genesis := Genesis(cbtx)

		fmt.Println("Genesis created")

		// 3. 存 block: key = hash, value = serialized block
		err := txn.Set(genesis.Hash, genesis.Serialize())
		utils.Handle(err)

		// 4. 存链尾指针
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})

	utils.Handle(err)

	return &BlockChain{lastHash, db}
}

// ContinueBlockChain：加载已有链
func ContinueBlockChain(address string) *BlockChain {

	// DB 不存在就不能加载
	if !utils.DBexists(dbFile) {
		fmt.Println("No existing blockchain found, create one first")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath
	opts.Logger = nil // 关闭 Badger 日志输出

	db, err := badger.Open(opts)
	utils.Handle(err)

	// 从 DB 读取 "lh"（last hash）
	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		utils.Handle(err)

		return item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})
	})

	utils.Handle(err)

	return &BlockChain{lastHash, db}
}

// ========================
// 添加新区块
// ========================

// AddBlock:
// 把交易打包进 block 并追加到链尾
func (chain *BlockChain) AddBlock(transactions []*Transaction) *Block {

	var lastHash []byte

	// 1. 读取当前链尾 hash
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})
	})
	utils.Handle(err)

	// 2. 创建新 block（指向旧链尾）
	newBlock := CreateBlock(transactions, lastHash)

	// 3. 写入 DB + 更新链尾
	err = chain.Database.Update(func(txn *badger.Txn) error {

		// 存 block
		if err := txn.Set(newBlock.Hash, newBlock.Serialize()); err != nil {
			return err
		}

		// 更新 last hash
		if err := txn.Set([]byte("lh"), newBlock.Hash); err != nil {
			return err
		}

		chain.LastHash = newBlock.Hash
		return nil
	})

	utils.Handle(err)

	return newBlock
}

// ========================
// 链遍历（核心：反向链）
// ========================

// Iterator：从最新 block 开始遍历
func (chain *BlockChain) Iterator() *BlockChainIterator {
	return &BlockChainIterator{chain.LastHash, chain.Database}
}

// Next：取当前 block，并移动到 prev block
func (iter *BlockChainIterator) Next() *Block {

	var block *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			block = Deserialize(val)
			return nil
		})
	})
	utils.Handle(err)

	// 指向前一个 block（链的关键）
	iter.CurrentHash = block.PrevHash

	return block
}

// ========================
// UTXO 查找（核心逻辑）
// ========================

// FindUTXO:
// 找所有“未花费输出”
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {

	UTXOs := make(map[string]TxOutputs)

	// 记录已经被花费的 output
	spendTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {

		block := iter.Next()

		for _, tx := range block.Transactions {

			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {

				// 如果这个 output 已经被花掉 → 跳过
				if spendTXOs[txID] != nil {
					for _, spentOut := range spendTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				// 收集未花费 output
				outs := UTXOs[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXOs[txID] = outs
			}

			// 记录 inputs（标记已花费 output）
			if !tx.IsCoinBase() {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spendTXOs[inTxID] = append(spendTXOs[inTxID], in.Out)
				}
			}
		}

		// 到 genesis block 停止
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return UTXOs
}

// ========================
// 交易查找
// ========================

// FindTransaction: 在整条链中找某个 tx
func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {

	iter := chain.Iterator()

	for {

		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Equal(tx.ID, ID) {
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction is not found")
}

// ========================
// 签名 / 验证（核心密码学）
// ========================

// SignTransaction:
// 用私钥对 transaction 签名
func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {

	prevTXs := make(map[string]Transaction)

	// 找所有 input 对应的 previous tx
	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		utils.Handle(err)
		prevTXs[hex.EncodeToString(in.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

// VerifyTransaction:
// 验证签名是否合法
func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		utils.Handle(err)
		prevTXs[hex.EncodeToString(in.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}
