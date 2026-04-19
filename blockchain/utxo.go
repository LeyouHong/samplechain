// 两条路径：
// 🟢 A. Reindex（全量重建）
// Blockchain → FindUTXO() → DB(UTXO)

// ✔ 不依赖 DB 旧状态
// ✔ 纯计算历史

// 🔵 B. Update（增量维护）
// 旧UTXO(DB)
//    ↓
// 删 inputs（spent）
//    ↓
// 加 outputs（new UTXO）

// ✔ 依赖 DB 当前状态
// ✔ 只处理新 block

package blockchain

import (
	"bytes"
	"encoding/hex"
	"log"

	"github.com/LeyouHong/samplechain/utils"
	"github.com/dgraph-io/badger"
)

// UTXO key 前缀（用于在 Badger 中区分 UTXO 数据）
var (
	utxoPrefix   = []byte("utxo-")
	prefixLength = len(utxoPrefix)
)

// UTXOSet：UTXO 索引层（不是链本身）
//
// 👉 作用：
// 不再全链扫描，而是直接从 DB 快速查找 UTXO
// UTXO Set（你现在的）
// DB 里始终维护：
// 当前所有未花费的输出（UTXO）
type UTXOSet struct {
	Blockchain *BlockChain
}

// =========================
// 查找可花费 UTXO
// =========================
// 找到足够金额的 UTXO，用于构造交易 inputs
func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {

	// txID -> output index 列表
	unspentOuts := make(map[string][]int)

	// 累计金额
	accumulated := 0

	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {

		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		// 遍历所有 UTXO
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {

			item := it.Item()

			// ⚠️ 修复点：使用 ValueCopy（避免 badger buffer 复用）
			v, err := item.ValueCopy(nil)
			utils.Handle(err)

			k := item.Key()
			k = bytes.TrimPrefix(k, utxoPrefix)

			txID := hex.EncodeToString(k)

			outs := DeserializeOutputs(v)

			// 遍历该 tx 的所有 output
			for outIdx, out := range outs.Outputs {

				// 判断是否属于当前用户
				if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOuts[txID] = append(unspentOuts[txID], outIdx)
				}
			}
		}

		return nil
	})

	utils.Handle(err)
	return accumulated, unspentOuts
}

// =========================
// 获取所有 UTXO
// =========================
// 用于查询余额
func (u UTXOSet) FindUTXO(pubKeyHash []byte) []TXOutput {

	var UTXOs []TXOutput
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {

		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {

			item := it.Item()

			// ⚠️ 修复点
			v, err := item.ValueCopy(nil)
			utils.Handle(err)

			outs := DeserializeOutputs(v)

			for _, out := range outs.Outputs {
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}

		return nil
	})

	utils.Handle(err)
	return UTXOs
}

// =========================
// 统计 UTXO 数量
// =========================
// debug / metrics 用
func (u UTXOSet) CountTransactions() int {

	db := u.Blockchain.Database
	counter := 0

	err := db.View(func(txn *badger.Txn) error {

		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			counter++
		}

		return nil
	})

	utils.Handle(err)
	return counter
}

// =========================
// 重建 UTXO 索引
// =========================
// ⚠️ 关键操作：
// 用全链扫描重新生成 UTXO DB
func (u UTXOSet) Reindex() {

	db := u.Blockchain.Database

	// 清空旧 UTXO
	u.DeleteByPrefix(utxoPrefix)

	// 从链重新计算 UTXO
	UTXO := u.Blockchain.FindUTXO()

	err := db.Update(func(txn *badger.Txn) error {

		for txId, outs := range UTXO {

			key, err := hex.DecodeString(txId)
			if err != nil {
				return err
			}

			key = append(utxoPrefix, key...)

			err = txn.Set(key, outs.Serialize())
			utils.Handle(err)
		}

		return nil
	})

	utils.Handle(err)
}

// =========================
// 更新 UTXO（核心逻辑）
// =========================
// 当新区块加入时：
// 1. 删除已花费 UTXO
// 2. 写入新 UTXO
func (u *UTXOSet) Update(block *Block) {

	db := u.Blockchain.Database

	err := db.Update(func(txn *badger.Txn) error {

		for _, tx := range block.Transactions {

			// ❗ Coinbase 没有 input，不消耗 UTXO
			if !tx.IsCoinBase() {

				for _, in := range tx.Inputs {

					updatedOuts := TxOutputs{}

					inID := append(utxoPrefix, in.ID...)

					item, err := txn.Get(inID)
					utils.Handle(err)

					// ⚠️ 修复点：ValueCopy
					v, err := item.ValueCopy(nil)
					utils.Handle(err)

					outs := DeserializeOutputs(v)

					// 删除已被消费的 output
					for outIdx, out := range outs.Outputs {
						if outIdx != in.Out {
							updatedOuts.Outputs = append(updatedOuts.Outputs, out)
						}
					}

					// 如果没有剩余 UTXO，删除 key
					if len(updatedOuts.Outputs) == 0 {
						if err := txn.Delete(inID); err != nil {
							utils.Handle(err)
						}
					} else {
						if err := txn.Set(inID, updatedOuts.Serialize()); err != nil {
							utils.Handle(err)
						}
					}
				}
			}

			// =========================
			// 写入新 UTXO（outputs）
			// =========================
			newOutputs := TxOutputs{}
			newOutputs.Outputs = append(newOutputs.Outputs, tx.Outputs...)

			txID := append(utxoPrefix, tx.ID...)

			if err := txn.Set(txID, newOutputs.Serialize()); err != nil {
				utils.Handle(err)
			}
		}

		return nil
	})

	utils.Handle(err)
}

// =========================
// 删除指定 prefix 的 key
// =========================
// 用于 Reindex / reset UTXO
func (u *UTXOSet) DeleteByPrefix(prefix []byte) {

	deleteKeys := func(keys [][]byte) error {

		return u.Blockchain.Database.Update(func(txn *badger.Txn) error {
			for _, key := range keys {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		})
	}

	const batchSize = 100000

	u.Blockchain.Database.View(func(txn *badger.Txn) error {

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		keys := make([][]byte, 0, batchSize)
		count := 0

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {

			key := it.Item().KeyCopy(nil)
			keys = append(keys, key)
			count++

			if count == batchSize {
				if err := deleteKeys(keys); err != nil {
					log.Panic(err)
				}
				keys = keys[:0]
				count = 0
			}
		}

		if count > 0 {
			if err := deleteKeys(keys); err != nil {
				log.Panic(err)
			}
		}

		return nil
	})
}
