package gguf

import "strings"

type Metadata struct {
	Architecture    string
	SplitCount      int64
	BlockCount      int64
	ContextLength   int64
	EmbeddingLength int64
	HeadCount       int64
	HeadCountKV     int64
	KVHeadsPerLayer bool
	KeyLength       int64
	ValueLength     int64
	SlidingWindow   int64
}

type KVShape struct {
	TotalKVHeads int64
	KeyLength    int64
	ValueLength  int64
	Sliding      bool
}

func (metadata Metadata) KVShape() (KVShape, bool) {
	if metadata.BlockCount <= 0 {
		return KVShape{}, false
	}
	headCountKV := metadata.HeadCountKV
	if headCountKV <= 0 {
		headCountKV = metadata.HeadCount
	}
	if headCountKV <= 0 {
		return KVShape{}, false
	}
	totalKVHeads := headCountKV
	if !metadata.KVHeadsPerLayer {
		totalKVHeads = headCountKV * metadata.BlockCount
	}
	keyLength, valueLength := metadata.KeyLength, metadata.ValueLength
	if keyLength <= 0 && metadata.HeadCount > 0 && metadata.EmbeddingLength > 0 {
		keyLength = metadata.EmbeddingLength / metadata.HeadCount
	}
	if valueLength <= 0 {
		valueLength = keyLength
	}
	if keyLength <= 0 {
		return KVShape{}, false
	}
	return KVShape{TotalKVHeads: totalKVHeads, KeyLength: keyLength, ValueLength: valueLength, Sliding: metadata.SlidingWindow > 0}, true
}

const (
	keyArchitecture = "general.architecture"
	keySplitCount   = "split.count"
)

var architectureIntegerSuffixes = map[string]func(*Metadata, int64){
	".block_count":                func(metadata *Metadata, value int64) { metadata.BlockCount = value },
	".context_length":             func(metadata *Metadata, value int64) { metadata.ContextLength = value },
	".embedding_length":           func(metadata *Metadata, value int64) { metadata.EmbeddingLength = value },
	".attention.head_count":       func(metadata *Metadata, value int64) { metadata.HeadCount = value },
	".attention.head_count_kv":    func(metadata *Metadata, value int64) { metadata.HeadCountKV = value },
	".attention.key_length":       func(metadata *Metadata, value int64) { metadata.KeyLength = value },
	".attention.value_length":     func(metadata *Metadata, value int64) { metadata.ValueLength = value },
	".attention.sliding_window":   func(metadata *Metadata, value int64) { metadata.SlidingWindow = value },
	".attention.head_count_kv[]":  func(metadata *Metadata, value int64) { metadata.HeadCountKV, metadata.KVHeadsPerLayer = value, true },
	".attention.head_count[]":     func(metadata *Metadata, value int64) { metadata.HeadCount = value },
	".attention.sliding_window[]": func(metadata *Metadata, value int64) { metadata.SlidingWindow = value },
}

type metadataCollector struct {
	architecture string
	splitCount   int64
	integers     map[string]int64
}

func newMetadataCollector() *metadataCollector {
	return &metadataCollector{integers: map[string]int64{}}
}

func (collector *metadataCollector) wantsString(key string) bool {
	return key == keyArchitecture
}

func (collector *metadataCollector) setString(key string, value string) {
	if key == keyArchitecture {
		collector.architecture = value
	}
}

func (collector *metadataCollector) setInteger(key string, value int64) {
	if key == keySplitCount {
		collector.splitCount = value
		return
	}
	if architectureSuffix(key) != "" {
		collector.integers[key] = value
	}
}

func (collector *metadataCollector) wantsIntegerArray(key string) bool {
	return architectureSuffix(key+"[]") != ""
}

func (collector *metadataCollector) setIntegerArraySum(key string, sum int64, count int64) {
	if strings.HasSuffix(key, ".attention.head_count") && count > 0 {
		sum /= count
	}
	if strings.HasSuffix(key, ".attention.sliding_window") && count > 0 {
		sum = min(sum, 1)
	}
	collector.integers[key+"[]"] = sum
}

func (collector *metadataCollector) metadata() Metadata {
	metadata := Metadata{Architecture: collector.architecture, SplitCount: collector.splitCount}
	prefix := collector.architecture
	if prefix == "" {
		return metadata
	}
	for key, value := range collector.integers {
		suffix, found := strings.CutPrefix(key, prefix)
		if !found {
			continue
		}
		if apply := architectureIntegerSuffixes[suffix]; apply != nil {
			apply(&metadata, value)
		}
	}
	return metadata
}

func architectureSuffix(key string) string {
	for suffix := range architectureIntegerSuffixes {
		if strings.HasSuffix(key, suffix) {
			return suffix
		}
	}
	return ""
}
