package blockchain

import (
	"fmt"
	"os"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func InitBlockChain() *BlockChain {
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		panic(err)
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte("lh"))

		if err == badger.ErrKeyNotFound {
			fmt.Println("No existing blockchain found")

			genesis := Genesis()

			fmt.Println("Genesis created")

			if err := txn.Set(genesis.Hash, genesis.Serialize()); err != nil {
				return err
			}

			if err := txn.Set([]byte("lh"), genesis.Hash); err != nil {
				return err
			}

			lastHash = genesis.Hash
			return nil
		}

		// exists case
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})
	})

	Handle(err)

	return &BlockChain{lastHash, db}
}
func (chain *BlockChain) AddBlock(data string) {
	var lastHash []byte

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
	Handle(err)

	newBlock := CreateBlock(data, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		if err := txn.Set(newBlock.Hash, newBlock.Serialize()); err != nil {
			return err
		}
		if err := txn.Set([]byte("lh"), newBlock.Hash); err != nil {
			return err
		}

		chain.LastHash = newBlock.Hash
		return nil
	})

	Handle(err)
}

func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}

	return iter
}

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
	Handle(err)

	iter.CurrentHash = block.PrevHash

	return block
}
