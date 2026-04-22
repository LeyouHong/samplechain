package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/LeyouHong/samplechain/blockchain"
)

// 网络协议与常量定义
const (
	protocol      = "tcp" // 使用 TCP 协议进行节点间通信
	version       = 1     // 当前协议版本号
	commandLength = 12    // 命令字段固定长度（字节数），不足 12 字节的命令用 0x0 填充
)

// 全局变量
var (
	nodeAddress     string                                    // 当前节点的地址（格式：localhost:PORT）
	mineAddress     string                                    // 矿工钱包地址（Base58 字符串），不为空时节点参与挖矿
	KnownNodes      = []string{"localhost:3000"}              // 已知节点列表，localhost:3000 为默认种子节点
	blocksInTransit = [][]byte{}                              // 正在同步中（待下载）的区块哈希列表
	memoryPool      = make(map[string]blockchain.Transaction) // 内存池：存放待打包的交易，key 为交易 ID 的十六进制字符串
	stateDB         *blockchain.StateDB                       // 账户状态层，在 StartServer 时初始化，供交易验证和挖矿使用
)

// ─── 消息结构体 ─────────────────────────────────────────────

// Addr 用于广播已知节点地址列表
type Addr struct {
	AddrList []string
}

// Block 用于向其他节点发送一个完整的区块
type Block struct {
	AddrFrom string // 发送方地址
	Block    []byte // 序列化后的区块数据
}

// GetBlocks 用于请求对方节点返回其所有区块哈希
type GetBlocks struct {
	AddrFrom string // 请求方地址
}

// GetData 用于请求对方节点返回某个具体的区块或交易
type GetData struct {
	AddrFrom string // 请求方地址
	Type     string // 数据类型："block" 或 "tx"
	ID       []byte // 目标区块哈希或交易 ID
}

// Inv（Inventory）用于告知对方自己持有哪些区块或交易
type Inv struct {
	AddrFrom string   // 发送方地址
	Type     string   // 数据类型："block" 或 "tx"
	Items    [][]byte // 区块哈希或交易 ID 列表
}

// Tx 用于向其他节点广播一笔交易
type Tx struct {
	AddrFrom    string // 发送方地址
	Transaction []byte // 序列化后的交易数据
}

// Version 用于节点握手，交换高度信息以决定谁需要同步区块
type Version struct {
	Version    int    // 协议版本号
	BestHeight int    // 发送方当前最长链的高度
	AddrFrom   string // 发送方地址
}

// ─── 命令编解码工具 ─────────────────────────────────────────

// CmdToBytes 将命令字符串编码为固定 commandLength 字节的字节数组
func CmdToBytes(cmd string) []byte {
	var bytes [commandLength]byte
	for i, c := range cmd {
		bytes[i] = byte(c)
	}
	return bytes[:]
}

// BytesToCmd 将固定长度的字节数组解码回命令字符串，过滤掉填充的 0x0 字节
func BytesToCmd(bytes []byte) string {
	var cmd []byte
	for _, b := range bytes {
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

// ExtractCmd 从原始请求中提取命令头部（前 commandLength 个字节）
func ExtractCmd(request []byte) []byte {
	return request[:commandLength]
}

// ─── 主动请求 ────────────────────────────────────────────────

// RequestBlocks 向所有已知节点发送 getblocks 请求，触发区块同步
func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlocks(node)
	}
}

// ─── 发送消息函数 ────────────────────────────────────────────

// SendAddr 向指定地址广播已知节点列表（包含自身地址）
func SendAddr(address string) {
	nodes := Addr{KnownNodes}
	nodes.AddrList = append(nodes.AddrList, nodeAddress)
	payload := GobEncode(nodes)
	request := append(CmdToBytes("addr"), payload...)
	SendData(address, request)
}

// SendBlock 向指定地址发送一个序列化后的区块
func SendBlock(addr string, b *blockchain.Block) {
	data := Block{nodeAddress, b.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("block"), payload...)
	SendData(addr, request)
}

// SendData 建立 TCP 连接并发送原始字节数据到目标地址
// 若连接失败，则将该节点从 KnownNodes 中移除
func SendData(addr string, data []byte) {
	conn, err := net.Dial(protocol, addr)
	if err != nil {
		fmt.Printf("%s is not available\n", addr)
		var updatedNodes []string
		for _, node := range KnownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node)
			}
		}
		KnownNodes = updatedNodes
		return
	}
	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(data))
	if err != nil {
		log.Panic(err)
	}
}

// SendInv 向指定地址发送 Inventory 消息，告知对方自己持有的区块或交易列表
func SendInv(address, kind string, items [][]byte) {
	inventory := Inv{nodeAddress, kind, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)
	SendData(address, request)
}

// SendGetBlocks 向指定地址请求其完整的区块哈希列表
func SendGetBlocks(address string) {
	payload := GobEncode(GetBlocks{nodeAddress})
	request := append(CmdToBytes("getblocks"), payload...)
	SendData(address, request)
}

// SendGetData 向指定地址请求某个具体的区块或交易数据
func SendGetData(address, kind string, id []byte) {
	payload := GobEncode(GetData{nodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)
	SendData(address, request)
}

// SendTx 向指定地址广播一笔交易
func SendTx(addr string, tnx *blockchain.Transaction) {
	data := Tx{nodeAddress, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)
	SendData(addr, request)
}

// SendVersion 向指定地址发送 Version 握手消息，携带本节点当前最高区块高度
func SendVersion(addr string, chain *blockchain.BlockChain) {
	bestHeight := chain.GetBestHeight()
	payload := GobEncode(Version{version, bestHeight, nodeAddress})
	request := append(CmdToBytes("version"), payload...)
	SendData(addr, request)
}

// ─── 消息处理函数 ────────────────────────────────────────────

// HandleAddr 处理收到的 addr 消息：将新地址合并到已知节点列表，并向所有节点请求区块
func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownNodes))
	RequestBlocks()
}

// HandleBlock 处理收到的 block 消息：反序列化区块并添加到本地链
// 账户模型下不再需要重建 UTXO 索引
// TODO: 区块同步完成后，需遍历区块内交易并 apply 到 stateDB 保持状态一致
func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	block := blockchain.Deserialize(payload.Block)

	fmt.Println("Recevied a new block!")
	chain.AddBlock(block)
	fmt.Printf("Added block %x\n", block.Hash)

	if len(blocksInTransit) > 0 {
		// 还有未下载的区块，继续请求队列中的下一个
		blockHash := blocksInTransit[0]
		SendGetData(payload.AddrFrom, "block", blockHash)
		blocksInTransit = blocksInTransit[1:]
	} else {
		// 所有区块同步完毕
		fmt.Println("All blocks synced")
	}
}

// HandleInv 处理收到的 inv 消息：根据类型分别处理区块或交易的清单
func HandleInv(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Inv

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("Recevied inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		// 处理空 inventory 的边界情况，避免节点卡死
		if len(payload.Items) == 0 {
			fmt.Println("No blocks in inventory, requesting again or waiting...")
			SendGetBlocks(payload.AddrFrom)
			return
		}

		blocksInTransit = payload.Items
		blockHash := payload.Items[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		// 将已请求的区块从待同步列表中移除
		var newInTransit [][]byte
		for _, b := range blocksInTransit {
			if !bytes.Equal(b, blockHash) {
				newInTransit = append(newInTransit, b)
			}
		}
		blocksInTransit = newInTransit
	}

	if payload.Type == "tx" {
		txID := payload.Items[0]
		// 若内存池中尚未有该交易，则向对方请求完整交易数据
		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			SendGetData(payload.AddrFrom, "tx", txID)
		}
	}
}

// HandleGetBlocks 处理收到的 getblocks 请求：将本地所有区块哈希以 inv 消息回复对方
func HandleGetBlocks(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blocks := chain.GetBlockHashes()
	SendInv(payload.AddrFrom, "block", blocks)
}

// HandleGetData 处理收到的 getdata 请求：根据类型返回对应的区块或交易
func HandleGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	if payload.Type == "block" {
		block, err := chain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}
		SendBlock(payload.AddrFrom, &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]
		SendTx(payload.AddrFrom, &tx)
	}
}

// HandleTx 处理收到的 tx 消息：将交易加入内存池，并根据节点角色决定后续行为
// - 若为种子节点（KnownNodes[0]）：将交易 ID 转发给其他节点
// - 若为矿工节点且内存池交易数 >= 2：触发打包挖矿
func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	tx := blockchain.DeserializeTransaction(payload.Transaction)
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	fmt.Printf("%s, %d\n", nodeAddress, len(memoryPool))

	if nodeAddress == KnownNodes[0] {
		// 当前节点是种子节点，负责将交易 ID 广播给其他节点
		for _, node := range KnownNodes {
			if node != nodeAddress && node != payload.AddrFrom {
				SendInv(node, "tx", [][]byte{tx.ID})
			}
		}
	} else {
		// 当前节点是矿工节点，内存池积累足够交易后开始挖矿
		if len(memoryPool) >= 2 && len(mineAddress) > 0 {
			MineTx(chain)
		}
	}
}

// MineTx 从内存池中取出有效交易，附加 coinbase 奖励交易后打包新区块
// 挖矿成功后广播新区块，清空已打包的交易；若内存池仍有剩余则递归继续挖矿
func MineTx(chain *blockchain.BlockChain) {
	var txs []*blockchain.Transaction

	// 用 stateDB 验证内存池中每笔交易的签名和 nonce，筛选出合法交易
	for id := range memoryPool {
		tx := memoryPool[id]
		if chain.VerifyTransaction(&tx, stateDB) {
			txs = append(txs, &tx)
		}
	}

	if len(txs) == 0 {
		fmt.Println("All Transactions are invalid")
		return
	}

	// mineAddress 是 Base58 字符串，转为 []byte 传给新版 CoinBaseTx
	cbTx := blockchain.CoinBaseTx([]byte(mineAddress), "")
	txs = append(txs, cbTx)

	// 挖出新区块，MineBlock 内部会执行状态变更（转账、nonce 更新）并持久化
	newBlock := chain.MineBlock(txs, stateDB)

	fmt.Println("New Block mined")

	// 将已打包的交易从内存池中删除
	for _, tx := range txs {
		delete(memoryPool, hex.EncodeToString(tx.ID))
	}

	// 将新区块哈希广播给所有其他已知节点
	for _, node := range KnownNodes {
		if node != nodeAddress {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	// 若内存池中还有交易，继续递归挖矿
	if len(memoryPool) > 0 {
		MineTx(chain)
	}
}

// HandleVersion 处理收到的 version 握手消息，通过比较区块高度决定同步方向
func HandleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	if bestHeight < otherHeight {
		// 对方链更长，向对方请求区块
		SendGetBlocks(payload.AddrFrom)
	} else if bestHeight > otherHeight {
		// 本地链更长，将版本信息发给对方让其同步
		SendVersion(payload.AddrFrom, chain)
	}

	// 若该节点尚未记录，则加入已知节点列表
	if !NodeIsKnown(payload.AddrFrom) {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
		fmt.Printf("Added new node: %s, known nodes: %v\n", payload.AddrFrom, KnownNodes)
	}
}

// HandleConnection 处理单个 TCP 连接：读取请求数据，解析命令头，分发到对应处理函数
func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	req, err := ioutil.ReadAll(conn)
	defer conn.Close()

	if err != nil {
		log.Panic(err)
	}

	command := BytesToCmd(req[:commandLength])
	fmt.Printf("Received %s command\n", command)

	switch command {
	case "addr":
		HandleAddr(req)
	case "block":
		HandleBlock(req, chain)
	case "inv":
		HandleInv(req, chain)
	case "getblocks":
		HandleGetBlocks(req, chain)
	case "getdata":
		HandleGetData(req, chain)
	case "tx":
		HandleTx(req, chain)
	case "version":
		HandleVersion(req, chain)
	default:
		fmt.Println("Unknown command")
	}
}

// StartServer 启动区块链节点服务器：
// 1. 初始化节点地址和矿工地址
// 2. 监听 TCP 端口
// 3. 加载本地区块链数据库，初始化 stateDB
// 4. 若不是种子节点，主动向种子节点发送 version 握手消息
// 5. 循环接受并发处理连接
func StartServer(nodeID, minerAddress string) {
	nodeAddress = fmt.Sprintf("localhost:%s", nodeID)
	mineAddress = minerAddress

	ln, err := net.Listen(protocol, nodeAddress)
	if err != nil {
		log.Panic(err)
	}
	defer ln.Close()

	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()

	// 初始化状态层，复用区块链数据库，通过 state: 前缀与区块数据隔离
	stateDB = blockchain.NewStateDB(chain.Database)

	go CloseDB(chain)

	if nodeAddress != KnownNodes[0] {
		SendVersion(KnownNodes[0], chain)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Panic(err)
		}
		go HandleConnection(conn, chain)
	}
}

// ─── 工具函数 ────────────────────────────────────────────────

// GobEncode 将任意 Go 数据结构使用 gob 编码序列化为字节切片
func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

// NodeIsKnown 检查给定地址是否已在已知节点列表中
func NodeIsKnown(addr string) bool {
	for _, node := range KnownNodes {
		if node == addr {
			return true
		}
	}
	return false
}

// CloseDB 在后台 goroutine 中监听系统信号（SIGINT / SIGTERM / os.Interrupt）
// 收到信号后优雅关闭区块链数据库并退出进程，防止数据损坏
func CloseDB(chain *blockchain.BlockChain) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	go func() {
		<-sigs
		log.Println("Shutting down blockchain node...")
		if chain.Database != nil {
			_ = chain.Database.Close()
		}
		os.Exit(0)
	}()
}
