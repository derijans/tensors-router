package gguf

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const ggufMagic = 0x46554747

type Limits struct {
	MaxKeyValues   uint64
	MaxKeyBytes    uint64
	MaxStringBytes uint64
	MaxArrayItems  uint64
	MaxHeaderBytes int64
}

func DefaultLimits() Limits {
	return Limits{MaxKeyValues: 8192, MaxKeyBytes: 256, MaxStringBytes: 1 << 20, MaxArrayItems: 4 << 20, MaxHeaderBytes: 64 << 20}
}

var (
	ErrNotGGUF            = errors.New("file is not GGUF")
	ErrUnsupportedVersion = errors.New("unsupported GGUF version")
	ErrLimitExceeded      = errors.New("GGUF header exceeds a reader limit")
)

type valueType uint32

const (
	typeUint8 valueType = iota
	typeInt8
	typeUint16
	typeInt16
	typeUint32
	typeInt32
	typeFloat32
	typeBool
	typeString
	typeArray
	typeUint64
	typeInt64
	typeFloat64
)

type headerReader struct {
	source *bufio.Reader
	limits Limits
	read   int64
}

func ReadMetadata(source io.Reader, limits Limits) (Metadata, error) {
	reader := &headerReader{source: bufio.NewReader(source), limits: limits}
	keyValueCount, err := reader.preamble()
	if err != nil {
		return Metadata{}, err
	}
	collected := newMetadataCollector()
	for range keyValueCount {
		key, err := reader.string(limits.MaxKeyBytes)
		if err != nil {
			return Metadata{}, err
		}
		kind, err := reader.uint32()
		if err != nil {
			return Metadata{}, err
		}
		if err := reader.value(collected, key, valueType(kind)); err != nil {
			return Metadata{}, fmt.Errorf("gguf key %q: %w", key, err)
		}
	}
	return collected.metadata(), nil
}

func (reader *headerReader) preamble() (uint64, error) {
	magic, err := reader.uint32()
	if err != nil || magic != ggufMagic {
		return 0, ErrNotGGUF
	}
	version, err := reader.uint32()
	if err != nil {
		return 0, err
	}
	if version != 2 && version != 3 {
		return 0, fmt.Errorf("%w %d", ErrUnsupportedVersion, version)
	}
	if _, err := reader.uint64(); err != nil {
		return 0, err
	}
	keyValueCount, err := reader.uint64()
	if err != nil {
		return 0, err
	}
	if keyValueCount > reader.limits.MaxKeyValues {
		return 0, ErrLimitExceeded
	}
	return keyValueCount, nil
}

func (reader *headerReader) value(collected *metadataCollector, key string, kind valueType) error {
	switch kind {
	case typeString:
		if !collected.wantsString(key) {
			return reader.skipString()
		}
		text, err := reader.string(reader.limits.MaxStringBytes)
		if err == nil {
			collected.setString(key, text)
		}
		return err
	case typeArray:
		return reader.array(collected, key)
	default:
		number, isInteger, err := reader.scalar(kind)
		if err == nil && isInteger {
			collected.setInteger(key, number)
		}
		return err
	}
}

func (reader *headerReader) array(collected *metadataCollector, key string) error {
	itemKind, err := reader.uint32()
	if err != nil {
		return err
	}
	count, err := reader.uint64()
	if err != nil {
		return err
	}
	if count > reader.limits.MaxArrayItems {
		return ErrLimitExceeded
	}
	keepIntegers := collected.wantsIntegerArray(key)
	var sum int64
	for range count {
		if valueType(itemKind) == typeString {
			if err := reader.skipString(); err != nil {
				return err
			}
			continue
		}
		if valueType(itemKind) == typeArray {
			return fmt.Errorf("nested GGUF arrays are not supported")
		}
		number, isInteger, err := reader.scalar(valueType(itemKind))
		if err != nil {
			return err
		}
		if keepIntegers && isInteger {
			if sum > math.MaxInt64-number {
				return ErrLimitExceeded
			}
			sum += number
		}
	}
	if keepIntegers {
		collected.setIntegerArraySum(key, sum, int64(count))
	}
	return nil
}

func (reader *headerReader) scalar(kind valueType) (int64, bool, error) {
	switch kind {
	case typeUint8, typeInt8, typeBool:
		value, err := reader.bytes(1)
		if err != nil {
			return 0, false, err
		}
		if kind == typeInt8 {
			return int64(int8(value[0])), true, nil
		}
		return int64(value[0]), kind != typeBool, nil
	case typeUint16, typeInt16:
		value, err := reader.bytes(2)
		if err != nil {
			return 0, false, err
		}
		if kind == typeInt16 {
			return int64(int16(binary.LittleEndian.Uint16(value))), true, nil
		}
		return int64(binary.LittleEndian.Uint16(value)), true, nil
	case typeUint32, typeInt32, typeFloat32:
		value, err := reader.uint32()
		if kind == typeInt32 {
			return int64(int32(value)), true, err
		}
		return int64(value), kind == typeUint32, err
	case typeUint64, typeInt64, typeFloat64:
		value, err := reader.uint64()
		if kind == typeUint64 && value > math.MaxInt64 {
			return 0, false, err
		}
		return int64(value), kind != typeFloat64, err
	default:
		return 0, false, fmt.Errorf("unknown GGUF value type %d", kind)
	}
}

func (reader *headerReader) string(limit uint64) (string, error) {
	length, err := reader.uint64()
	if err != nil {
		return "", err
	}
	if length > limit {
		return "", ErrLimitExceeded
	}
	value, err := reader.bytes(int64(length))
	return strings.Clone(string(value)), err
}

func (reader *headerReader) skipString() error {
	length, err := reader.uint64()
	if err != nil {
		return err
	}
	if length > reader.limits.MaxStringBytes {
		return ErrLimitExceeded
	}
	return reader.discard(int64(length))
}

func (reader *headerReader) uint32() (uint32, error) {
	value, err := reader.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(value), nil
}

func (reader *headerReader) uint64() (uint64, error) {
	value, err := reader.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(value), nil
}

func (reader *headerReader) bytes(count int64) ([]byte, error) {
	if err := reader.account(count); err != nil {
		return nil, err
	}
	buffer := make([]byte, count)
	_, err := io.ReadFull(reader.source, buffer)
	return buffer, err
}

func (reader *headerReader) discard(count int64) error {
	if err := reader.account(count); err != nil {
		return err
	}
	_, err := io.CopyN(io.Discard, reader.source, count)
	return err
}

func (reader *headerReader) account(count int64) error {
	if count < 0 || count > reader.limits.MaxHeaderBytes-reader.read {
		return ErrLimitExceeded
	}
	reader.read += count
	return nil
}
