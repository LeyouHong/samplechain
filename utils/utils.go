package utils

import (
	"log"
	"os"
)

func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// DBexists 判断区块链数据库是否已经存在
func DBexists(dbFile string) bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}
