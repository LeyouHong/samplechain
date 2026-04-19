// 每个 block 都会单独生成一棵 Merkle Tree
// Blockchain（链）
// 是 block 的链式结构
// 用 PrevHash 连接
// ✔ Block（区块）
// 只包含自己的 transactions
// 有自己的 Merkle Tree
// ✔ Merkle Tree
// 只负责“这个 block 内部的 transactions”

package blockchain

import "crypto/sha256"

// MerkleTree represents a binary hash tree used to compress
// all transactions in a block into a single Merkle Root.
type MerkleTree struct {
	RootNode *MerkleNode // root of the tree (Merkle Root)
}

// MerkleNode represents a node in the Merkle Tree.
// Leaf nodes contain transaction hashes.
// Non-leaf nodes contain combined hashes of children.
type MerkleNode struct {
	Left  *MerkleNode // left child node
	Right *MerkleNode // right child node
	Data  []byte      // hash value stored in this node
}

// NewMerkleNode creates a new Merkle node.
// If it is a leaf node (no children), it hashes the data directly.
// If it is an internal node, it hashes the concatenation of left + right child hashes.
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	// Leaf node: hash raw transaction data
	if left == nil && right == nil {
		hash := sha256.Sum256(data)
		node.Data = hash[:]
	} else {
		// Internal node: hash(left + right)
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:]
	}

	node.Left = left
	node.Right = right

	return &node
}

// NewMerkleTree builds a Merkle Tree from a list of transaction data.
// Step 1: Create leaf nodes from raw data
// Step 2: Iteratively build upper levels until root is reached
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	// If odd number of transactions, duplicate last one
	// (Bitcoin-style padding rule)
	if len(data)%2 != 0 {
		data = append(data, data[len(data)-1])
	}

	// Create leaf nodes
	for _, datum := range data {
		node := NewMerkleNode(nil, nil, datum)
		nodes = append(nodes, *node)
	}

	// Build tree upward until root remains
	for len(nodes) > 1 {
		var newLevel []MerkleNode

		// Pair nodes and build parent nodes
		for j := 0; j < len(nodes); j += 2 {
			node := NewMerkleNode(&nodes[j], &nodes[j+1], nil)
			newLevel = append(newLevel, *node)
		}

		nodes = newLevel
	}

	// Final remaining node is the Merkle Root
	return &MerkleTree{RootNode: &nodes[0]}
}
