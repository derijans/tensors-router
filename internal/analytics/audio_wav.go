package analytics

import (
	"bytes"
	"encoding/binary"
)

const (
	riffHeaderSize        = 12
	riffChunkHeaderSize   = 8
	waveFormatByteRateEnd = 12
	streamedWaveDataSize  = 0xFFFFFFFF
)

func waveDurationSeconds(prefix []byte, totalBytes int64) float64 {
	if len(prefix) < riffHeaderSize || !bytes.Equal(prefix[0:4], []byte("RIFF")) || !bytes.Equal(prefix[8:12], []byte("WAVE")) {
		return 0
	}
	byteRate := uint32(0)
	for chunkStart := riffHeaderSize; chunkStart+riffChunkHeaderSize <= len(prefix); {
		chunkID := prefix[chunkStart : chunkStart+4]
		chunkSize := binary.LittleEndian.Uint32(prefix[chunkStart+4 : chunkStart+8])
		payloadStart := chunkStart + riffChunkHeaderSize
		switch {
		case bytes.Equal(chunkID, []byte("fmt ")) && payloadStart+waveFormatByteRateEnd <= len(prefix):
			byteRate = binary.LittleEndian.Uint32(prefix[payloadStart+8 : payloadStart+12])
		case bytes.Equal(chunkID, []byte("data")):
			return waveDataSeconds(int64(chunkSize), int64(payloadStart), totalBytes, byteRate)
		}
		chunkStart = payloadStart + int(chunkSize) + int(chunkSize%2)
	}
	return 0
}

func waveDataSeconds(declaredSize int64, dataOffset int64, totalBytes int64, byteRate uint32) float64 {
	if byteRate == 0 {
		return 0
	}
	deliveredSize := totalBytes - dataOffset
	dataSize := declaredSize
	if declaredSize == 0 || declaredSize == streamedWaveDataSize || declaredSize > deliveredSize {
		dataSize = deliveredSize
	}
	if dataSize <= 0 {
		return 0
	}
	return float64(dataSize) / float64(byteRate)
}
