package merkle

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

// Proof представляет доказательство включения для листа
type Proof struct {
	LeafHash []byte
	Path     []ProofStep
}

// ProofStep представляет шаг в доказательстве (хэш и направление)
type ProofStep struct {
	Hash   []byte
	IsLeft bool // true, если хэш находится слева
}

// GenerateProof генерирует доказательство включения для листа с заданным индексом
func (t *MerkleTree) GenerateProof(leafIndex int) (*Proof, error) {
	if leafIndex < 0 || leafIndex >= len(t.Leaves) {
		return nil, fmt.Errorf("invalid leaf index: %d", leafIndex)
	}

	proof := &Proof{
		LeafHash: t.Leaves[leafIndex].Hash,
		Path:     []ProofStep{},
	}

	current := t.findLeafNode(leafIndex)
	if current == nil {
		return nil, fmt.Errorf("leaf node not found for index: %d", leafIndex)
	}

	// Поднимаемся по дереву до корня
	for current.Parent != nil {
		parent := current.Parent
		if parent.Left == current {
			// Если текущий узел — левый, добавляем хэш правого узла
			proof.Path = append(proof.Path, ProofStep{
				Hash:   parent.Right.Hash,
				IsLeft: false,
			})
		} else {
			// Если текущий узел — правый, добавляем хэш левого узла
			proof.Path = append(proof.Path, ProofStep{
				Hash:   parent.Left.Hash,
				IsLeft: true,
			})
		}
		current = parent
	}

	return proof, nil
}

// VerifyProof проверяет доказательство включения
func (p *Proof) VerifyProof(rootHash []byte) bool {
	currentHash := p.LeafHash
	for _, step := range p.Path {
		if step.IsLeft {
			// Хэш шага слева
			combined := append(step.Hash, currentHash...)
			hash := sha256.Sum256(combined)
			currentHash = hash[:]
		} else {
			// Хэш шага справа
			combined := append(currentHash, step.Hash...)
			hash := sha256.Sum256(combined)
			currentHash = hash[:]
		}
	}
	return bytes.Equal(currentHash, rootHash)
}
