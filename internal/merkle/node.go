package merkle

import (
	"crypto/sha256"
)

// Node представляет узел дерева Меркла
type Node struct {
	Hash   []byte
	Left   *Node
	Right  *Node
	Data   []byte // Для листовых узлов
	Parent *Node
}

// NewLeafNode создает новый листовой узел
func NewLeafNode(data []byte) *Node {
	hash := sha256.Sum256(data)
	return &Node{
		Hash: hash[:],
		Data: data,
	}
}

// NewParentNode создает новый родительский узел
func NewParentNode(left, right *Node) *Node {
	combined := append(left.Hash, right.Hash...)
	hash := sha256.Sum256(combined)
	return &Node{
		Hash:  hash[:],
		Left:  left,
		Right: right,
	}
}
