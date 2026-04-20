package wallet

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"

	"github.com/LeyouHong/samplechain/utils"
)

const walletFile = "./tmp/wallets_%s.data"

type Wallets struct {
	Wallets map[string]*Wallet
}

// 初始化
func CreateWallets(nodeId string) (*Wallets, error) {
	ws := Wallets{}
	ws.Wallets = make(map[string]*Wallet)

	err := ws.LoadFile(nodeId)
	return &ws, err
}

// 添加钱包
func (ws *Wallets) AddWallet() string {
	wallet := MakeWallet()
	address := string(wallet.Address())

	ws.Wallets[address] = wallet
	return address
}

// 获取所有地址
func (ws *Wallets) GetAllAddresses() []string {
	addresses := []string{}

	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}
	return addresses
}

// 获取钱包
func (ws Wallets) GetWallet(address string) Wallet {
	return *ws.Wallets[address]
}

// 加载文件（关键修复）
func (ws *Wallets) LoadFile(nodeId string) error {
	path := fmt.Sprintf(walletFile, nodeId)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// ✅ 第一次运行正常
		return nil
	}

	var wallets Wallets

	fileContent, err := os.ReadFile(path)
	utils.Handle(err)

	decoder := gob.NewDecoder(bytes.NewReader(fileContent))
	err = decoder.Decode(&wallets)
	utils.Handle(err)

	ws.Wallets = wallets.Wallets
	return nil
}

// 保存文件（关键修复）
func (ws *Wallets) SaveFile(nodeId string) {
	path := fmt.Sprintf(walletFile, nodeId)
	var content bytes.Buffer

	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(ws)
	utils.Handle(err)

	// ✅ 确保目录存在
	os.MkdirAll("./tmp", 0755)

	err = os.WriteFile(path, content.Bytes(), 0644)
	utils.Handle(err)
}
