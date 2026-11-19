package gguf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

type headerBuilder struct {
	keyCount uint64
	body     bytes.Buffer
}

func (builder *headerBuilder) writeString(target *bytes.Buffer, value string) {
	_ = binary.Write(target, binary.LittleEndian, uint64(len(value)))
	target.WriteString(value)
}

func (builder *headerBuilder) stringValue(key string, value string) *headerBuilder {
	builder.keyCount++
	builder.writeString(&builder.body, key)
	_ = binary.Write(&builder.body, binary.LittleEndian, uint32(typeString))
	builder.writeString(&builder.body, value)
	return builder
}

func (builder *headerBuilder) uint32Value(key string, value uint32) *headerBuilder {
	builder.keyCount++
	builder.writeString(&builder.body, key)
	_ = binary.Write(&builder.body, binary.LittleEndian, uint32(typeUint32))
	_ = binary.Write(&builder.body, binary.LittleEndian, value)
	return builder
}

func (builder *headerBuilder) int32ArrayValue(key string, values ...int32) *headerBuilder {
	builder.keyCount++
	builder.writeString(&builder.body, key)
	_ = binary.Write(&builder.body, binary.LittleEndian, uint32(typeArray))
	_ = binary.Write(&builder.body, binary.LittleEndian, uint32(typeInt32))
	_ = binary.Write(&builder.body, binary.LittleEndian, uint64(len(values)))
	for _, value := range values {
		_ = binary.Write(&builder.body, binary.LittleEndian, value)
	}
	return builder
}

func (builder *headerBuilder) stringArrayValue(key string, values ...string) *headerBuilder {
	builder.keyCount++
	builder.writeString(&builder.body, key)
	_ = binary.Write(&builder.body, binary.LittleEndian, uint32(typeArray))
	_ = binary.Write(&builder.body, binary.LittleEndian, uint32(typeString))
	_ = binary.Write(&builder.body, binary.LittleEndian, uint64(len(values)))
	for _, value := range values {
		builder.writeString(&builder.body, value)
	}
	return builder
}

func (builder *headerBuilder) bytes() []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint32(ggufMagic))
	_ = binary.Write(&out, binary.LittleEndian, uint32(3))
	_ = binary.Write(&out, binary.LittleEndian, uint64(291))
	_ = binary.Write(&out, binary.LittleEndian, builder.keyCount)
	out.Write(builder.body.Bytes())
	return out.Bytes()
}

func llamaHeader() *headerBuilder {
	return (&headerBuilder{}).
		uint32Value("llama.block_count", 32).
		stringArrayValue("tokenizer.ggml.tokens", "a", "b", "c").
		stringValue("tokenizer.chat_template", "{{ messages }}").
		stringValue("general.architecture", "llama").
		uint32Value("llama.context_length", 8192).
		uint32Value("llama.embedding_length", 4096).
		uint32Value("llama.attention.head_count", 32).
		uint32Value("llama.attention.head_count_kv", 8)
}

func TestReadMetadataResolvesArchitectureKeysWrittenBeforeTheArchitecture(t *testing.T) {
	metadata, err := ReadMetadata(bytes.NewReader(llamaHeader().bytes()), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Architecture != "llama" || metadata.BlockCount != 32 || metadata.HeadCountKV != 8 || metadata.ContextLength != 8192 {
		t.Fatalf("metadata=%+v", metadata)
	}
	shape, ok := metadata.KVShape()
	if !ok || shape.TotalKVHeads != 256 || shape.KeyLength != 128 || shape.ValueLength != 128 || shape.Sliding {
		t.Fatalf("kv shape=%+v ok=%t", shape, ok)
	}
}

func TestReadMetadataSumsPerLayerKVHeads(t *testing.T) {
	header := (&headerBuilder{}).
		stringValue("general.architecture", "hybrid").
		uint32Value("hybrid.block_count", 4).
		uint32Value("hybrid.attention.head_count", 16).
		uint32Value("hybrid.embedding_length", 2048).
		int32ArrayValue("hybrid.attention.head_count_kv", 8, 0, 8, 0)
	metadata, err := ReadMetadata(bytes.NewReader(header.bytes()), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	shape, ok := metadata.KVShape()
	if !ok || shape.TotalKVHeads != 16 {
		t.Fatalf("shape=%+v ok=%t, want 16 heads across the attention layers only", shape, ok)
	}
}

func TestReadMetadataIgnoresKeysOfAnotherArchitecture(t *testing.T) {
	header := (&headerBuilder{}).stringValue("general.architecture", "qwen3").uint32Value("llama.block_count", 99)
	metadata, err := ReadMetadata(bytes.NewReader(header.bytes()), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.BlockCount != 0 {
		t.Fatalf("block count=%d leaked from another architecture", metadata.BlockCount)
	}
}

func TestReadMetadataRejectsHugeDeclaredCountsWithoutReadingThem(t *testing.T) {
	var header bytes.Buffer
	_ = binary.Write(&header, binary.LittleEndian, uint32(ggufMagic))
	_ = binary.Write(&header, binary.LittleEndian, uint32(3))
	_ = binary.Write(&header, binary.LittleEndian, uint64(0))
	_ = binary.Write(&header, binary.LittleEndian, uint64(1<<60))
	if _, err := ReadMetadata(&header, DefaultLimits()); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err=%v, want ErrLimitExceeded", err)
	}

	oversized := (&headerBuilder{}).stringValue("general.architecture", string(make([]byte, 300)))
	limits := DefaultLimits()
	limits.MaxStringBytes = 200
	if _, err := ReadMetadata(bytes.NewReader(oversized.bytes()), limits); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err=%v, want ErrLimitExceeded for an oversized string", err)
	}
}

func TestReadMetadataStopsAtTheHeaderByteBudget(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxHeaderBytes = 64
	if _, err := ReadMetadata(bytes.NewReader(llamaHeader().bytes()), limits); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err=%v, want the header budget to stop the scan", err)
	}
}

func TestReadMetadataRejectsTruncatedAndForeignFiles(t *testing.T) {
	full := llamaHeader().bytes()
	if _, err := ReadMetadata(bytes.NewReader(full[:len(full)-3]), DefaultLimits()); err == nil {
		t.Fatal("a truncated header was accepted")
	}
	if _, err := ReadMetadata(bytes.NewReader([]byte("PK\x03\x04not a model")), DefaultLimits()); !errors.Is(err, ErrNotGGUF) {
		t.Fatalf("err=%v, want ErrNotGGUF", err)
	}
}

func FuzzReadMetadata(f *testing.F) {
	f.Add(llamaHeader().bytes())
	f.Add([]byte("GGUF"))
	f.Fuzz(func(t *testing.T, data []byte) {
		limits := DefaultLimits()
		limits.MaxHeaderBytes = int64(len(data)) + 1024
		_, _ = ReadMetadata(bytes.NewReader(data), limits)
	})
}
