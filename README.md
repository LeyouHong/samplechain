# 📦 samplechain

A simplified Bitcoin-like blockchain implementation in Go, built as a foundation for future DApp development.

---

## 🚀 Overview

This project implements core blockchain primitives including:

- Block structure with Proof of Work (PoW)
- UTXO-based transaction model
- ECDSA wallet with Base58Check addresses
- Transaction signing & verification
- Persistent storage via BadgerDB
- UTXO indexing with Merkle Tree
- P2P node networking with block/transaction propagation

---

## 🧱 Architecture

| Layer       | Components                               |
| ----------- | ---------------------------------------- |
| Blockchain  | Block, PoW, chain linking, iterator      |
| Transaction | UTXO model, coinbase, inputs/outputs     |
| Wallet      | ECDSA key gen, address creation, signing |
| Storage     | BadgerDB persistence                     |
| UTXO Index  | Spend tracking, reindex, update          |
| Merkle Tree | Transaction hashing for block integrity  |
| Network     | P2P TCP node, block/tx propagation       |

---

## 📡 Node Networking

Nodes communicate over TCP using a custom binary protocol. Supported message types:

`version` · `addr` · `getblocks` · `inv` · `getdata` · `block` · `tx`

### Start a node

```bash
# Start the seed node (port 3000)
NODE_ID=3000 go run main.go startnode

# Start a mining node
NODE_ID=4000 go run main.go startnode -miner <MINER_ADDRESS>

# Start a regular node
NODE_ID=5000 go run main.go startnode
```

Nodes automatically sync with the seed node (`localhost:3000`) on startup and propagate new transactions and blocks across the network.

---

## 💻 CLI Commands

### Create a blockchain

```bash
NODE_ID=3000 go run main.go createblockchain -address <ADDRESS>
```

### Create a wallet

```bash
NODE_ID=3000 go run main.go createwallet
```

### List addresses

```bash
NODE_ID=3000 go run main.go listaddresses
```

### Get balance

```bash
NODE_ID=3000 go run main.go getbalance -address <ADDRESS>
```

### Send a transaction

```bash
# Broadcast to network (requires a running node)
NODE_ID=3000 go run main.go send -from <FROM> -to <TO> -amount <AMOUNT>

# Mine immediately on this node
NODE_ID=3000 go run main.go send -from <FROM> -to <TO> -amount <AMOUNT> -mine
```

### Print the chain

```bash
NODE_ID=3000 go run main.go printchain
```

### Rebuild UTXO index

```bash
NODE_ID=3000 go run main.go reindexutxo
```

---

## 🔧 Tech Stack

- **Language**: Go
- **Storage**: [BadgerDB](https://github.com/dgraph-io/badger)
- **Crypto**: ECDSA (P-256), SHA-256, RIPEMD-160
- **Encoding**: GOB, Base58Check
- **Networking**: Raw TCP with custom protocol
