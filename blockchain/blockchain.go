package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/LeyouHong/samplechain/utils"
	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks_%s"                // BadgerDB 数据库路径模板，%s 由节点 ID 填充，实现多节点隔离存储
	genesisData = "First Transaction from Genesis" // 创世区块 coinbase 交易的附加数据
)

// BlockChain 表示一条区块链
// 通过 BadgerDB 持久化存储区块数据，LastHash 始终指向最长链的链尾区块哈希
type BlockChain struct {
	LastHash []byte     // 当前最长链末尾区块的哈希，用于快速定位链尾
	Database *badger.DB // BadgerDB 实例，key 为区块哈希，value 为序列化的区块数据；"lh" 键存储最新哈希
}

// DBexists 检查指定路径下的 BadgerDB 数据库是否已存在
// 通过判断 MANIFEST 文件是否存在来确认数据库是否初始化过
func DBexists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}

	return true
}

// ContinueBlockChain 加载已有的区块链数据库并返回 BlockChain 实例
// 若数据库不存在则打印提示并退出程序（使用 runtime.Goexit 保证 defer 正常执行）
func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if DBexists(path) == false {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(path)
	opts.Dir = path
	opts.ValueDir = path
	opts.Logger = nil // 关闭 BadgerDB 内部日志，减少控制台噪音

	db, err := openDB(path, opts)
	utils.Handle(err)

	// 从数据库中读取 "lh"（last hash）键，获取当前最长链的链尾哈希
	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		utils.Handle(err)
		lastHash, err = item.ValueCopy(nil)

		return err
	})
	utils.Handle(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

// InitBlockChain 初始化一条全新的区块链
// 创建创世区块并写入数据库，同时设置 "lh" 键指向创世区块哈希
// 若数据库已存在则打印提示并退出，防止重复初始化覆盖数据
func InitBlockChain(address, nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if DBexists(path) {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}
	var lastHash []byte
	opts := badger.DefaultOptions(path)
	opts.Dir = path
	opts.ValueDir = path
	opts.Logger = nil

	db, err := openDB(path, opts)
	utils.Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		// 创建 coinbase 交易和创世区块
		cbtx := CoinBaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis created")

		// 以区块哈希为 key 存储序列化的区块数据
		err = txn.Set(genesis.Hash, genesis.Serialize())
		utils.Handle(err)
		// 更新 "lh" 键指向创世区块
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})

	utils.Handle(err)

	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

// AddBlock 将一个已验证的区块写入数据库
// 若区块已存在则跳过；若新区块高度大于当前最长链，则更新 "lh" 使其成为新的链尾
// 此设计支持多节点同步时处理分叉，始终保留最长链
func (chain *BlockChain) AddBlock(block *Block) {
	err := chain.Database.Update(func(txn *badger.Txn) error {
		// 幂等检查：区块已存在则直接返回，避免重复写入
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		utils.Handle(err)

		// 读取当前链尾区块，比较高度决定是否更新最长链指针
		item, err := txn.Get([]byte("lh"))
		utils.Handle(err)
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		utils.Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock := Deserialize(lastBlockData)

		// 新区块高度更大，说明它在更长的链上，更新 "lh" 指向新区块
		if block.Height > lastBlock.Height {
			err = txn.Set([]byte("lh"), block.Hash)
			utils.Handle(err)
			chain.LastHash = block.Hash
		}

		return nil
	})
	utils.Handle(err)
}

// GetBestHeight 返回当前最长链的链尾区块高度
// 通过读取 "lh" 键定位链尾区块，再取其 Height 字段
func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		utils.Handle(err)
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		utils.Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock = *Deserialize(lastBlockData)

		return nil
	})
	utils.Handle(err)

	return lastBlock.Height
}

// GetBlock 根据区块哈希从数据库中查询并返回对应区块
// 若区块不存在则返回错误，供网络层在处理 getdata 请求时调用
func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block is not found")
		} else {
			blockData, _ := item.ValueCopy(nil)
			block = *Deserialize(blockData)
		}
		return nil
	})
	if err != nil {
		return block, err
	}

	return block, nil
}

// GetBlockHashes 从链尾向创世区块遍历，收集并返回所有区块的哈希列表
// 用于响应其他节点的 getblocks 请求，帮助对方判断自己缺少哪些区块
func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		blocks = append(blocks, block.Hash)

		// PrevHash 为空说明已到达创世区块，遍历结束
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

// MineBlock 验证交易、打包新区块并持久化到数据库
// 流程：验证所有交易合法性 → 读取链尾信息 → PoW 挖矿 → 写入数据库并更新链尾
func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	// 打包前逐一验证交易签名，拒绝无效交易
	for _, tx := range transactions {
		if chain.VerifyTransaction(tx) != true {
			log.Panic("Invalid Transaction")
		}
	}

	// 读取当前链尾哈希和高度，新区块高度 = 链尾高度 + 1
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		utils.Handle(err)
		lastHash, err = item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		utils.Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock := Deserialize(lastBlockData)
		lastHeight = lastBlock.Height

		return err
	})
	utils.Handle(err)

	// 执行 PoW 挖矿，创建新区块
	newBlock := CreateBlock(transactions, lastHash, lastHeight+1)

	// 将新区块写入数据库，并更新 "lh" 指向新区块
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		utils.Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})
	utils.Handle(err)

	return newBlock
}

// FindUTXO 遍历整条链，找出所有未被花费的交易输出（UTXO）
// 返回 map[交易ID字符串]TxOutputs，供 UTXOSet 初始化和重建索引时使用
// 算法：从链尾向创世区块遍历，记录已花费输出，跳过已花费的，收集剩余未花费输出
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int) // 记录已花费的输出：key 为交易 ID，value 为已花费的输出索引列表

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				// 若该输出索引已在已花费列表中，则跳过
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				// 该输出未被花费，加入 UTXO 集合
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}
			// coinbase 交易没有输入，无需记录已花费输出
			if tx.IsCoinBase() == false {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return UTXO
}

// FindTransaction 在整条链上按交易 ID 查找交易
// 从链尾向创世区块遍历，找到后立即返回；遍历完未找到则返回错误
func (bc *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := bc.Iterator()

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

	return Transaction{}, errors.New("Transaction does not exist")
}

// SignTransaction 为交易的每个输入收集其引用的历史交易，然后调用 tx.Sign 完成签名
// 签名前必须拿到被引用的历史交易，因为签名数据中包含被引用输出的 PubKeyHash
func (bc *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	// 收集所有输入引用的历史交易
	for _, in := range tx.Inputs {
		prevTX, err := bc.FindTransaction(in.ID)
		utils.Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

// VerifyTransaction 验证一笔交易的所有输入签名是否合法
// coinbase 交易直接返回 true；普通交易需收集历史交易后调用 tx.Verify
func (bc *BlockChain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinBase() {
		return true
	}
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := bc.FindTransaction(in.ID)
		utils.Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}

// retry 处理 BadgerDB 因异常退出遗留 LOCK 文件导致无法打开的情况
// 删除 LOCK 文件后以 Truncate 模式重新打开，恢复数据库可用性
func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	retryOpts := originalOpts
	retryOpts.Truncate = true // 截断损坏的 value log，优先保证数据库可打开
	db, err := badger.Open(retryOpts)
	return db, err
}

// openDB 打开 BadgerDB 数据库，并自动处理 LOCK 文件残留问题
// 若首次打开失败且错误信息包含 "LOCK"，则调用 retry 尝试恢复
func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		return db, nil
	}
}
