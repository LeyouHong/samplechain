package blockchain

import (
	"bytes"
	"fmt"

	"github.com/dgraph-io/badger"
)

// ==========================
// 🧠 Debug 工具：打印 BadgerDB 所有数据
// ==========================

// DumpDB 打印整个数据库（适用于 samplechain）
func (chain *BlockChain) DumpDB() {

	err := chain.Database.View(func(txn *badger.Txn) error {

		fmt.Println("========== BADGER DB DUMP ==========")

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {

			item := it.Item()
			key := item.Key()

			err := item.Value(func(val []byte) error {

				fmt.Printf("\n🔑 KEY: %s\n", string(key))

				// ==========================
				// 🧠 特殊 key 处理
				// ==========================

				// 👉 链尾指针
				if bytes.Equal(key, []byte("lh")) {
					fmt.Printf("📌 LastHash: %x\n", val)
					return nil
				}

				// 👉 UTXO 数据
				if bytes.HasPrefix(key, []byte("utxo-")) {
					fmt.Printf("💰 UTXO RAW: %x\n", val)

					outs := DeserializeOutputs(val)

					for i, out := range outs.Outputs {
						fmt.Printf("   └─ Output %d: value=%d pubKeyHash=%x\n",
							i, out.Value, out.PubKeyHash)
					}

					return nil
				}

				// 👉 Block 数据（默认情况）
				fmt.Printf("📦 VALUE (raw): %x\n", val)

				return nil
			})

			if err != nil {
				return err
			}
		}

		fmt.Println("========== END DUMP ==========")

		return nil
	})

	if err != nil {
		fmt.Println("DumpDB error:", err)
	}
}
