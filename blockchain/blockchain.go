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
// 数据库相关常量
// ========================

// dbPath: 区块数据存储目录
// dbFile: 用于判断 DB 是否存在（Badger 会生成 MANIFEST 文件）
// genesisData: 创世区块默认数据
const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis"
)

// ========================
// 核心结构定义
// ========================

// BlockChain 表示整条链
// LastHash：当前链尾区块 hash（快速定位最新区块）
// Database：底层 KV 存储（BadgerDB）
type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// BlockChainIterator 用于遍历区块链（从尾到头）
// CurrentHash：当前指向的区块
type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// ========================
// 初始化 & 加载区块链
// ========================

// InitBlockChain
// 初始化区块链（只允许第一次创建）
//
// 流程：
// 1. 如果 DB 已存在 → 直接退出
// 2. 创建 coinbase 交易
// 3. 创建创世区块
// 4. 存入 DB：
//   - key = block hash → value = block data
//   - key = "lh" → value = latest hash
func InitBlockChain(address string) *BlockChain {
	if utils.DBexists(dbFile) {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	utils.Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		// 创世交易（挖矿奖励）
		cbtx := CoinBaseTx(address, genesisData)

		// 创世区块
		genesis := Genesis(cbtx)

		fmt.Println("Genesis created")

		// 存 block
		err := txn.Set(genesis.Hash, genesis.Serialize())
		utils.Handle(err)

		// 存链尾指针（lh = last hash）
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})

	utils.Handle(err)

	return &BlockChain{lastHash, db}
}

// ContinueBlockChain
// 加载已有区块链
//
// 核心：从 DB 中读取 "lh"（链尾 hash）
func ContinueBlockChain(address string) *BlockChain {
	if !utils.DBexists(dbFile) {
		fmt.Println("No existing blockchain found, create one first")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	utils.Handle(err)

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
// 区块操作
// ========================

// AddBlock
// 向链尾添加新区块
//
// 流程：
// 1. 读取当前链尾 hash
// 2. 创建新区块
// 3. 写入 DB
// 4. 更新链尾指针
func (chain *BlockChain) AddBlock(transactions []*Transaction) {
	var lastHash []byte

	// 读链尾
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

	// 创建新区块
	newBlock := CreateBlock(transactions, lastHash)

	// 写入 DB
	err = chain.Database.Update(func(txn *badger.Txn) error {
		// 存 block 数据
		if err := txn.Set(newBlock.Hash, newBlock.Serialize()); err != nil {
			return err
		}

		// 更新链尾
		if err := txn.Set([]byte("lh"), newBlock.Hash); err != nil {
			return err
		}

		chain.LastHash = newBlock.Hash
		return nil
	})

	utils.Handle(err)
}

// ========================
// 迭代器
// ========================

// Iterator 返回一个从链尾开始的迭代器
func (chain *BlockChain) Iterator() *BlockChainIterator {
	return &BlockChainIterator{chain.LastHash, chain.Database}
}

// Next
// 返回当前区块，并移动到前一个区块
//
// 本质：
// current → prev → prev → ...
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

	// 指向前一个区块
	iter.CurrentHash = block.PrevHash

	return block
}

// ========================
// UTXO 相关逻辑（重点）
// ========================

// FindUnspentTransactions
// 查找 address 所有“未花费交易”
//
// 核心思想：
// 1. 从链尾往前扫
// 2. 记录已花费的 output
// 3. 剩下的就是 UTXO
func (chain *BlockChain) FindUnspentTransactions(pubKeyHash []byte) []Transaction {
	var unspentTXs []Transaction

	// key: txID → value: 已花费的 output index
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {

				// 如果这个 output 已经被花费 → 跳过
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				// 属于当前 address → 加入未花费列表
				if out.IsLockedWithKey(pubKeyHash) {
					unspentTXs = append(unspentTXs, *tx)
				}
			}

			// 非 coinbase 才有 input
			if !tx.IsCoinBase() {
				for _, in := range tx.Inputs {
					if in.UsesKey(pubKeyHash) {
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}

		// 到创世区块结束
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return unspentTXs
}

// FindUTXO
// 从未花费交易中提取所有属于 address 的 output
func (chain *BlockChain) FindUTXO(pubKeyHash []byte) []TXOutput {
	var UTXOs []TXOutput

	unspentTransactions := chain.FindUnspentTransactions(pubKeyHash)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Outputs {
			if out.IsLockedWithKey(pubKeyHash) {
				UTXOs = append(UTXOs, out)
			}
		}
	}

	return UTXOs
}

// FindSpendableOutputs
// 找到足够支付 amount 的 UTXO
//
// 返回：
// accumulated：累计金额
// map[txID][]outIdx：可用输出
func (chain *BlockChain) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {

	unspentOutputs := make(map[string][]int)
	unspentTXs := chain.FindUnspentTransactions(pubKeyHash)

	accumulated := 0

Work:
	for _, tx := range unspentTXs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs {
			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOutputs
}

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

func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		utils.Handle(err)
		prevTXs[hex.EncodeToString(in.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		utils.Handle(err)
		prevTXs[hex.EncodeToString(in.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}
