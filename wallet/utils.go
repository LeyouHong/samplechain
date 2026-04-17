package wallet

import (
	"log"

	"github.com/mr-tron/base58"
)

// Base58Encode 将字节数组编码为 Base58 字符串
func Base58Encode(input []byte) []byte {
	encode := base58.Encode(input)
	return []byte(encode)
}

// Base58Decode 将 Base58 字符串解码为字节数组
func Base58Decode(input []byte) []byte {
	decoded, err := base58.Decode(string(input[:]))
	if err != nil {
		log.Panic(err)
	}
	return decoded
}
