package blockchain

import (
	"bytes"
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

// ─── 数据库工具 ──────────────────────────────────────────────

// DBexists 检查指定路径下的 BadgerDB 数据库是否已存在
// 通过判断 MANIFEST 文件是否存在来确认数据库是否初始化过
func DBexists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
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

// ─── 链的初始化与加载 ────────────────────────────────────────

// ContinueBlockChain 加载已有的区块链数据库并返回 BlockChain 实例
// 若数据库不存在则打印提示并退出程序（使用 runtime.Goexit 保证 defer 正常执行）
func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if !DBexists(path) {
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

	return &BlockChain{lastHash, db}
}

// InitBlockChain 初始化一条全新的区块链
// 创建创世区块并写入数据库，同时设置 "lh" 键指向创世区块哈希
// 若数据库已存在则打印提示并退出，防止重复初始化覆盖数据
// address：初始矿工地址（Base58 字节字符串），用于接收创世区块的 coinbase 奖励
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
		// address 是 Base58 字符串，转为 []byte 传给新版 CoinBaseTx
		cbtx := CoinBaseTx([]byte(address), genesisData)
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

	return &BlockChain{lastHash, db}
}

// ─── 区块操作 ────────────────────────────────────────────────

// AddBlock 将一个已验证的区块写入数据库
// 若区块已存在则跳过；若新区块高度大于当前最长链，则更新 "lh" 使其成为新的链尾
// 此设计支持多节点同步时处理分叉，始终保留最长链
func (chain *BlockChain) AddBlock(block *Block) {
	err := chain.Database.Update(func(txn *badger.Txn) error {
		// 幂等检查：区块已存在则直接返回，避免重复写入
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		err := txn.Set(block.Hash, block.Serialize())
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
		item, err := txn.Get(blockHash)
		if err != nil {
			return errors.New("Block is not found")
		}
		blockData, _ := item.ValueCopy(nil)
		block = *Deserialize(blockData)
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

// ─── 挖矿 ────────────────────────────────────────────────────

// MineBlock 验证交易、打包新区块并持久化到数据库
// stateDB：当前状态层，用于验证交易签名和执行状态变更
// 流程：验证所有交易 → 读取链尾信息 → PoW 挖矿 → 执行状态变更 → 写入数据库
func (chain *BlockChain) MineBlock(transactions []*Transaction, stateDB *StateDB) *Block {
	var lastHash []byte
	var lastHeight int

	// 打包前逐一验证交易，拒绝签名非法的交易
	for _, tx := range transactions {
		if !chain.VerifyTransaction(tx, stateDB) {
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

	// 执行区块内所有交易的状态变更（转账、合约部署/调用）
	chain.applyTransactions(transactions, stateDB)

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

// applyTransactions 依次执行区块内每笔交易，将结果写入状态层
// coinbase：向矿工地址增加余额
// Transfer：扣除发送方余额，增加接收方余额，nonce +1
// Deploy / Call：留给后续接入 EVM 时实现
func (chain *BlockChain) applyTransactions(transactions []*Transaction, stateDB *StateDB) {
	for _, tx := range transactions {
		switch tx.Type {

		case TxTypeTransfer:
			if tx.IsCoinBase() {
				// coinbase 交易：直接向矿工发放奖励，无需扣款
				stateDB.AddBalance(tx.To, tx.Value)
			} else {
				// 普通转账：扣发送方，加接收方，nonce +1
				stateDB.SubBalance(tx.From, tx.Value)
				stateDB.AddBalance(tx.To, tx.Value)
				stateDB.IncrementNonce(tx.From)
			}

		case TxTypeDeploy:
			// TODO: 接入 EVM 后在此执行合约部署逻辑
			stateDB.IncrementNonce(tx.From)

		case TxTypeCall:
			// TODO: 接入 EVM 后在此执行合约调用逻辑
			stateDB.IncrementNonce(tx.From)
		}
	}
}

// ─── 交易签名与验证 ──────────────────────────────────────────

// SignTransaction 使用私钥对交易进行签名
// 账户模型下无需收集历史交易，直接签名即可
func (bc *BlockChain) SignTransaction(tx *Transaction, privKey interface{}) {
	// 断言为 ecdsa.PrivateKey 后调用 tx.Sign
	// 此处保留方法是为了和 network.go 的调用保持兼容
	// 实际签名逻辑已内聚到 Transaction.Sign()，建议直接调用
	fmt.Println("Use tx.Sign(privKey) directly in account model")
}

// VerifyTransaction 验证一笔交易的签名是否合法
// coinbase 交易直接返回 true
// 普通交易从状态层读取发送方公钥，调用 tx.Verify 验证签名
func (bc *BlockChain) VerifyTransaction(tx *Transaction, stateDB *StateDB) bool {
	if tx.IsCoinBase() {
		return true
	}

	// 从状态层读取发送方账户，取出注册时存入的公钥
	account := stateDB.GetAccount(tx.From)
	if len(account.PublicKey) == 0 {
		log.Printf("VerifyTransaction: no public key found for %x", tx.From)
		return false
	}

	// 验证 nonce：防止重放攻击
	if tx.Nonce != account.Nonce {
		log.Printf("VerifyTransaction: nonce mismatch, expected %d got %d", account.Nonce, tx.Nonce)
		return false
	}

	return tx.Verify(account.PublicKey)
}

// ─── 迭代器 ─────────────────────────────────────────────────

// Iterator 返回一个从链尾向创世区块方向遍历的迭代器
func (chain *BlockChain) Iterator() *BlockChainIterator {
	return &BlockChainIterator{chain.LastHash, chain.Database}
}

// BlockChainIterator 区块链迭代器，从链尾向创世区块方向遍历
type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Next 返回当前哈希对应的区块，并将指针移向前一个区块
func (iter *BlockChainIterator) Next() *Block {
	var block *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		utils.Handle(err)
		data, _ := item.ValueCopy(nil)
		block = Deserialize(data)
		return nil
	})
	utils.Handle(err)

	iter.CurrentHash = block.PrevHash
	return block
}

// ─── 工具方法 ────────────────────────────────────────────────

// FindTransaction 在整条链上按交易 ID 查找交易
// 主要供调试和区块浏览器使用；正常业务流程通过 StateDB 查账户状态即可
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
