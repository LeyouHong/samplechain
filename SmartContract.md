# 方案三：混合模型 —— UTXO + 智能合约

## 整体思路

保留现有的 UTXO 转账逻辑**完全不动**，在旁边新建一套合约层，两者通过交易类型字段区分，共享同一个 BadgerDB 和同一套网络/共识代码。

```
现有代码                    新增代码
─────────────────────────  ─────────────────────────
UTXO 转账                   合约部署 / 调用
TXInput / TXOutput          ContractAccount / Storage
MineBlock (转账验证)         EVM 执行引擎
BadgerDB (区块存储)          BadgerDB (状态存储，同一个DB)
```

---

## 第一步：扩展交易结构

现有的 `Transaction` 只能表达转账，需要加一个类型字段和合约专用字段：

```go
// transaction.go

const (
    TxTypeTransfer = 0  // 普通 UTXO 转账（现有逻辑不变）
    TxTypeDeploy   = 1  // 部署新合约
    TxTypeCall     = 2  // 调用已有合约
)

type Transaction struct {
    ID      []byte
    Type    int         // ← 新增：交易类型
    Inputs  []TXInput
    Outputs []TXOutput

    // 合约专用字段，Type=0 时全为空
    From     []byte  // 调用方地址
    To       []byte  // 合约地址（Deploy 时为空，调用时填）
    Value    int64   // 随调用附带的原生币数量
    Data     []byte  // Deploy 时是合约字节码，Call 时是 ABI 编码的调用数据
    GasLimit uint64  // 愿意消耗的最大 Gas
}
```

`Type=0` 时走现有 UTXO 逻辑，`Type=1/2` 时走新的 EVM 逻辑，两条路完全隔离。

---

## 第二步：合约账户与状态存储

合约需要存两类数据：**账户信息**（代码、余额）和**合约内部存储**（变量状态）。

```go
// contract_account.go  （新建文件）

type ContractAccount struct {
    Address  []byte
    Balance  int64
    CodeHash []byte  // 合约字节码的哈希
    Nonce    uint64  // 防重放
}
```

存储结构设计（复用现有的 BadgerDB）：

```
// 合约账户信息
key:  "contract:account:{地址hex}"
val:  ContractAccount 序列化

// 合约字节码
key:  "contract:code:{CodeHash hex}"
val:  字节码原文

// 合约内部存储（每个变量槽一条记录）
key:  "contract:storage:{地址hex}:{slot hex}"
val:  32字节数据
```

新建一个 `StateDB` 结构封装这些操作：

```go
// statedb.go  （新建文件）

type StateDB struct {
    db *badger.DB
}

func (s *StateDB) GetCode(addr []byte) []byte { ... }
func (s *StateDB) SetCode(addr []byte, code []byte) { ... }
func (s *StateDB) GetStorage(addr []byte, slot []byte) []byte { ... }
func (s *StateDB) SetStorage(addr []byte, slot []byte, val []byte) { ... }
func (s *StateDB) GetBalance(addr []byte) int64 { ... }
func (s *StateDB) SetBalance(addr []byte, val int64) { ... }
```

---

## 第三步：接入 go-ethereum EVM

只引入 EVM 执行包，不引入以太坊其他任何东西：

```bash
go get github.com/ethereum/go-ethereum@latest
```

新建 `evm_executor.go`，核心是实现 `StateDB` 接口让 EVM 能读写状态层：

```go
// evm_executor.go  （新建文件）

type EVMExecutor struct {
    stateDB *StateDB
}

// ExecuteDeploy 部署合约，返回合约地址
func (e *EVMExecutor) ExecuteDeploy(tx *Transaction) (contractAddr []byte, err error) {
    // 1. 构造 EVM 运行环境（区块上下文、链规则）
    // 2. 把 tx.Data（字节码）送入 EVM 执行
    // 3. EVM 执行构造函数，返回合约运行时代码
    // 4. 将运行时代码存入 StateDB
    // 5. 返回生成的合约地址（由 From + Nonce 派生）
}

// ExecuteCall 调用合约，返回执行结果
func (e *EVMExecutor) ExecuteCall(tx *Transaction) (result []byte, err error) {
    // 1. 从 StateDB 读取目标合约代码
    // 2. 构造 EVM 运行环境
    // 3. 把 tx.Data（ABI 编码的函数调用）送入 EVM
    // 4. EVM 执行过程中对 Storage 的读写自动走 StateDB
    // 5. 返回执行结果和 Gas 消耗
}
```

EVM 需要实现一个 `vm.StateDB` 接口（约20个方法），本质上就是把 go-ethereum 的接口方法一一对应到 `StateDB` 的读写操作上。

---

## 第四步：改造 MineBlock

这是把两套系统串起来的关键位置：

```go
// blockchain.go  MineBlock 改造

func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
    stateDB := NewStateDB(chain.Database)
    executor := NewEVMExecutor(stateDB)

    for _, tx := range transactions {
        switch tx.Type {

        case TxTypeTransfer:
            // 走现有逻辑，验证 UTXO 签名
            if !chain.VerifyTransaction(tx) {
                log.Panic("Invalid Transaction")
            }

        case TxTypeDeploy:
            // 部署合约
            addr, err := executor.ExecuteDeploy(tx)
            if err != nil {
                log.Printf("Deploy failed: %v, skip", err)
                continue
            }
            log.Printf("Contract deployed at %x", addr)

        case TxTypeCall:
            // 调用合约
            result, err := executor.ExecuteCall(tx)
            if err != nil {
                log.Printf("Call failed: %v, skip", err)
                continue
            }
            log.Printf("Call result: %x", result)
        }
    }

    // 后续 PoW 挖矿、写库逻辑不变
    // ...
}
```

---

## 第五步：暴露 JSON-RPC 给 DApp

DApp 用 `ethers.js` 连接节点，需要实现几个接口。新建 `rpc_server.go`：

```go
// rpc_server.go  （新建文件）

// 启动一个 HTTP 服务，监听 8545 端口（以太坊标准端口）
func StartRPCServer(chain *BlockChain, stateDB *StateDB) {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // 解析 JSON-RPC 请求，分发到对应处理函数
    })
    http.ListenAndServe(":8545", nil)
}

// 需要实现的最小接口集合：
// eth_sendRawTransaction    → 接收 DApp 发来的签名交易，放入 memoryPool
// eth_call                  → 只读调用合约，不上链
// eth_getTransactionReceipt → 查询交易执行结果
// eth_getLogs               → 查询合约事件日志
// eth_blockNumber           → 返回当前链高度
// eth_getBalance            → 查询账户余额
```

DApp 端连接方式：

```javascript
// 前端代码
const provider = new ethers.JsonRpcProvider("http://localhost:8545");
const contract = new ethers.Contract(contractAddress, abi, provider);
await contract.someMethod();
```

---

## 改动量评估

| 模块                  | 改动类型 | 说明                                    |
| --------------------- | -------- | --------------------------------------- |
| `transaction.go`      | 修改     | 加 Type 和合约字段，现有逻辑不动        |
| `blockchain.go`       | 修改     | MineBlock 加 switch 分支                |
| `network.go`          | 不需要改 | —                                       |
| `block.go`            | 可选修改 | 可选加 StateRoot 字段                   |
| `statedb.go`          | 新建     | 约 150 行                               |
| `contract_account.go` | 新建     | 约 50 行                                |
| `evm_executor.go`     | 新建     | 约 200 行（主要是实现 vm.StateDB 接口） |
| `rpc_server.go`       | 新建     | 约 200 行                               |

**现有代码几乎不动，新增约 600 行**，大部分是模板代码。最难的部分是实现 `vm.StateDB` 接口，但 go-ethereum 的文档和示例很完整。

---

## 建议的开发顺序

```
1. 扩展 Transaction 结构，确保序列化兼容      （半天）
2. 实现 StateDB，跑通读写测试                 （1天）
3. 实现 EVM 接口，跑通一个最简合约部署         （2天）
4. 改造 MineBlock，联调转账+合约共存           （1天）
5. 实现 JSON-RPC，用 ethers.js 跑通端到端     （2天）
```

完成后即可得到一条**既支持 UTXO 转账又支持 Solidity 合约**的链，前端可以直接使用以太坊生态的所有工具。
