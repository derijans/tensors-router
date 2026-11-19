package memoryfit

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

type Combo struct {
	NodeID         string
	WeightsHash    string
	BackendMode    string
	GPULayers      int
	KVTypeK        string
	KVTypeV        string
	FlashAttention bool
	TensorSplit    string
	ContextSize    int
}

func (combo Combo) Learnable() bool {
	return strings.TrimSpace(combo.WeightsHash) != "" && strings.TrimSpace(combo.NodeID) != ""
}

func (combo Combo) Key() string {
	return digest(combo.placementFields(), strconv.Itoa(combo.ContextSize))
}

func (combo Combo) PlacementKey() string {
	return digest(combo.placementFields())
}

func (combo Combo) placementFields() string {
	return strings.Join([]string{
		combo.NodeID,
		combo.WeightsHash,
		combo.BackendMode,
		strconv.Itoa(combo.GPULayers),
		combo.KVTypeK,
		combo.KVTypeV,
		strconv.FormatBool(combo.FlashAttention),
		combo.TensorSplit,
	}, "\x1f")
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1e")))
	return hex.EncodeToString(sum[:])
}
