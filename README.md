# 📦 samplechain

A simplified blockchain implementation in Go.

This project is my foundation for future DApp development.

---

## 🚀 Overview

This project implements a simplified Bitcoin-like blockchain system including:

- Block structure + Proof of Work
- UTXO transaction model
- Wallet (ECDSA-based)
- Transaction signing & verification
- Persistent storage (BadgerDB)
- Chain traversal
- UTXO indexing system

---

## 🧱 Architecture

- **Blockchain layer**: Block, PoW, chain linking
- **Transaction layer**: UTXO model
- **Wallet layer**: Key generation, address creation
- **Storage layer**: BadgerDB persistence
- **UTXO layer**: spend tracking & indexing

---

## 💡 Features

### ⛓ Blockchain Core

- Create genesis block
- Add new blocks
- Proof of Work (PoW)
- Block hashing
- Chain traversal (iterator)

---

### 💸 Transaction System (UTXO Model)

- Create transactions
- Coinbase (mining reward) transactions
- Input/Output model
- UTXO selection (FindSpendableOutputs)
- Change output handling

---

### 🔐 Wallet System

- ECDSA key pair generation
- Public key hashing (SHA256 + RIPEMD160)
- Base58Check address generation
- Private key serialization (x509)

---

### ✍️ Digital Signature

- Transaction signing (ECDSA)
- Per-input signature
- Signature verification
- Prev transaction reconstruction

---

### 🗂 UTXO System

- UTXO indexing in BadgerDB
- Find all spendable outputs
- Reindex full UTXO set
- Update UTXO after block mining

---

### 💾 Persistent Storage (BadgerDB)

- Store blocks by hash
- Store latest hash pointer
- Store UTXO set with prefix indexing
- Block reloading from disk

---

## 📡 Blockchain Commands

### 🧾 Create blockchain

```bash
go run main.go createblockchain <address>
```

---

### ➕ Add block

```bash
go run main.go addblock
```

---

### 💸 Send transaction

```bash
go run main.go send <from> <to> <amount>
```

---

### 🔍 Print blockchain

```bash
go run main.go printchain
```

---

### 🧮 Rebuild UTXO index

```bash
go run main.go reindex
```

---

## 📊 Core APIs

### Blockchain

- InitBlockChain(address string)
- ContinueBlockChain(address string)
- AddBlock(transactions)
- Iterator()
- FindTransaction(ID)
- SignTransaction(tx, privKey)
- VerifyTransaction(tx)

---

### Transaction

- NewTransaction(from, to, amount, chain)
- CoinBaseTx(to, data)
- Hash()
- Serialize()
- Sign()
- Verify()
- TrimmedCopy()
- IsCoinBase()

---

### Wallet

- MakeWallet()
- NewKeyPair()
- Address()
- PublicKeyHash()
- ValidateAddress()
- BytesToPrivateKey()

---

### UTXO Set

- FindSpendableOutputs()
- FindUTXO()
- Update(block)
- Reindex()
- DeleteByPrefix()

---

## 🧠 Design Notes

- UTXO model (Bitcoin-style)
- ECDSA (P256) for signatures
- Hash160: SHA256 + RIPEMD160
- Base58Check address format
- BadgerDB for persistence

---

## ⚠️ Current Limitations

- No P2P networking
- No mempool
- No smart contract VM
- Simplified signature format
- Single-node execution

---

## 🚀 Future Work

- P2P networking layer
- Merkle Tree integration
- Smart contract VM (WASM or stack-based)
- Mempool system
- Fee mechanism
- Multi-node consensus simulation
