package blockchain

import (
	"crypto/sha256"
	"fmt"
	"log"

	"github.com/dgraph-io/badger"
)

// BadgerDB 中的 key 前缀，与区块数据隔离
const (
	accountPrefix = "state:account:" // 账户信息：state:account:{地址hex}
	codePrefix    = "state:code:"    // 合约字节码：state:code:{CodeHash hex}
	storagePrefix = "state:storage:" // 合约存储槽：state:storage:{地址hex}:{slot hex}
)

// StateDB 管理所有账户的状态读写
// 复用区块链已有的 BadgerDB 实例，通过 key 前缀与区块数据隔离
type StateDB struct {
	db *badger.DB
}

// NewStateDB 创建一个 StateDB 实例
func NewStateDB(db *badger.DB) *StateDB {
	return &StateDB{db: db}
}

// ─── 账户操作 ────────────────────────────────────────────────

// GetAccount 根据地址读取账户，不存在时返回一个余额为 0 的新账户
func (s *StateDB) GetAccount(address []byte) *Account {
	key := []byte(accountPrefix + fmt.Sprintf("%x", address))
	var account *Account

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			// 账户不存在，返回空账户（首次使用的地址）
			account = &Account{Address: address, Balance: 0, Nonce: 0}
			return nil
		}
		if err != nil {
			return err
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		account = DeserializeAccount(data)
		return nil
	})
	if err != nil {
		log.Panic(err)
	}
	return account
}

// SetAccount 将账户写入数据库
func (s *StateDB) SetAccount(account *Account) {
	key := []byte(accountPrefix + fmt.Sprintf("%x", account.Address))
	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, account.Serialize())
	})
	if err != nil {
		log.Panic(err)
	}
}

// ─── 余额操作 ────────────────────────────────────────────────

// GetBalance 返回地址的当前余额
func (s *StateDB) GetBalance(address []byte) int64 {
	return s.GetAccount(address).Balance
}

// SetBalance 设置地址的余额
func (s *StateDB) SetBalance(address []byte, balance int64) {
	account := s.GetAccount(address)
	account.Balance = balance
	s.SetAccount(account)
}

// AddBalance 增加地址的余额（用于收款和挖矿奖励）
func (s *StateDB) AddBalance(address []byte, amount int64) {
	account := s.GetAccount(address)
	account.Balance += amount
	s.SetAccount(account)
}

// SubBalance 减少地址的余额（用于转账和支付 Gas）
// 余额不足时直接 panic，调用前应先用 GetBalance 检查
func (s *StateDB) SubBalance(address []byte, amount int64) {
	account := s.GetAccount(address)
	if account.Balance < amount {
		log.Panic("StateDB: insufficient balance")
	}
	account.Balance -= amount
	s.SetAccount(account)
}

// ─── Nonce 操作 ──────────────────────────────────────────────

// GetNonce 返回地址的当前 nonce
func (s *StateDB) GetNonce(address []byte) uint64 {
	return s.GetAccount(address).Nonce
}

// IncrementNonce 将地址的 nonce +1，每次成功发出交易后调用
func (s *StateDB) IncrementNonce(address []byte) {
	account := s.GetAccount(address)
	account.Nonce++
	s.SetAccount(account)
}

// ─── 合约代码操作 ────────────────────────────────────────────

// GetCode 根据 CodeHash 读取合约字节码
func (s *StateDB) GetCode(codeHash []byte) []byte {
	key := []byte(codePrefix + fmt.Sprintf("%x", codeHash))
	var code []byte

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		code, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}
	return code
}

// SetCode 存储合约字节码，并将 CodeHash 写入对应账户
func (s *StateDB) SetCode(address []byte, code []byte) {
	// 计算字节码哈希
	hash := sha256.Sum256(code)
	codeHash := hash[:]

	// 存储字节码
	key := []byte(codePrefix + fmt.Sprintf("%x", codeHash))
	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, code)
	})
	if err != nil {
		log.Panic(err)
	}

	// 更新账户的 CodeHash 字段，标记为合约账户
	account := s.GetAccount(address)
	account.CodeHash = codeHash
	s.SetAccount(account)
}

// ─── 合约存储操作 ────────────────────────────────────────────

// GetStorage 读取合约地址在指定存储槽的值
// slot 是 32 字节的存储槽索引，对应 Solidity 中的状态变量槽位
func (s *StateDB) GetStorage(address []byte, slot []byte) []byte {
	key := []byte(storagePrefix + fmt.Sprintf("%x:%x", address, slot))
	var value []byte

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			// 未初始化的槽默认为 32 字节零值
			value = make([]byte, 32)
			return nil
		}
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}
	return value
}

// SetStorage 写入合约地址在指定存储槽的值
func (s *StateDB) SetStorage(address []byte, slot []byte, value []byte) {
	key := []byte(storagePrefix + fmt.Sprintf("%x:%x", address, slot))
	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
	if err != nil {
		log.Panic(err)
	}
}
