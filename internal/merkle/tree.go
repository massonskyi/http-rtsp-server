package merkle

import (
	"fmt"
)

// MerkleTree представляет дерево Меркла
type MerkleTree struct {
	Root   *Node
	Leaves []*Node
}

// NewMerkleTree создает новое дерево Меркла из списка блоков данных
func NewMerkleTree(dataBlocks [][]byte) (*MerkleTree, error) {
	if len(dataBlocks) == 0 {
		return nil, fmt.Errorf("no data blocks provided")
	}

	// Создаем листья (хэши блоков данных)
	leaves := make([]*Node, len(dataBlocks))
	for i, block := range dataBlocks {
		leaves[i] = NewLeafNode(block)
	}

	// Строим дерево
	root := buildTree(leaves)

	// Устанавливаем родительские связи
	setParents(root, nil)

	return &MerkleTree{
		Root:   root,
		Leaves: leaves,
	}, nil
}

// buildTree рекурсивно строит дерево Меркла
func buildTree(nodes []*Node) *Node {
	if len(nodes) == 1 {
		return nodes[0]
	}

	var nextLevel []*Node
	for i := 0; i < len(nodes); i += 2 {
		if i+1 < len(nodes) {
			// Если есть пара, создаем родительский узел
			parent := NewParentNode(nodes[i], nodes[i+1])
			nextLevel = append(nextLevel, parent)
		} else {
			// Если остался один узел, просто добавляем его
			nextLevel = append(nextLevel, nodes[i])
		}
	}

	return buildTree(nextLevel)
}

// setParents устанавливает родительские связи для узлов
func setParents(node *Node, parent *Node) {
	node.Parent = parent
	if node.Left != nil {
		setParents(node.Left, node)
	}
	if node.Right != nil {
		setParents(node.Right, node)
	}
}

// findLeafNode находит листовой узел по индексу
func (t *MerkleTree) findLeafNode(leafIndex int) *Node {
	if leafIndex < 0 || leafIndex >= len(t.Leaves) {
		return nil
	}
	return t.Leaves[leafIndex]
}
