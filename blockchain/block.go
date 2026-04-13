package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
)

type Block struct {
	Hash     []byte
	PrevHash []byte
	Data     []byte
	Nonce    int
}

func CreateBlock(data string, prevHash []byte) *Block {
	b := &Block{[]byte{}, prevHash, []byte(data), 0}

	pow := NewProofOfWork(b)
	nonce, hash := pow.Run()
	b.Hash = hash[:]
	b.Nonce = nonce

	return b
}

func Genesis() *Block {
	return CreateBlock("Genesis Block", []byte{})
}

func (b *Block) Serialize() []byte {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)

	err := encoder.Encode(b)

	Handle(err)

	return result.Bytes()
}

func Deserialize(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&block)

	Handle(err)

	return &block
}

func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
