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
- Merkle Tree

---

## 🧱 Architecture

- **Blockchain layer**: Block, PoW, chain linking
- **Transaction layer**: UTXO model
- **Wallet layer**: Key generation, address creation
- **Storage layer**: BadgerDB persistence
- **UTXO layer**: spend tracking & indexing
- **Merkle Tree**: Transaction hashing tree for efficient inclusion proof

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

### Merkle Tree

- Transaction hashing for inclusion proof
- Build Merkle root for each block
- Verify transaction integrity efficiently

---

## 📡 Blockchain Commands

### 🧾 Create blockchain

```bash
go run main.go createblockchain -address <ADDRESS>
```

---

### 💸 Send transaction

```bash
go run main.go send -from <FROM> -to <TO> -amount <AMOUNT>
```

---

### 🔍 Print blockchain

```bash
go run main.go printchain
```

---

### 🧮 Rebuild UTXO index

```bash
go run main.go reindexutxo
```

---

### 🔍 Get Balance

```bash
go run main.go getbalance -address <ADDRESS>
```

---

### 🧾 Create wallet

```bash
go run main.go createwallet
```

---

### 🔍 List Address

```bash
go run main.go listaddresses
```

---

### 🔍 Dump DB

```bash
go run main.go dumpdb
```

---
