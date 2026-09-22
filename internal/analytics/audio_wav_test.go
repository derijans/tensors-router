package analytics

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func pcmWave(sampleRate uint32, channels uint16, bitsPerSample uint16, declaredDataSize uint32, samples int) []byte {
	blockAlign := channels * bitsPerSample / 8
	var wave bytes.Buffer
	wave.WriteString("RIFF")
	_ = binary.Write(&wave, binary.LittleEndian, uint32(0))
	wave.WriteString("WAVE")
	wave.WriteString("LIST")
	_ = binary.Write(&wave, binary.LittleEndian, uint32(3))
	wave.Write([]byte{'a', 'b', 'c', 0})
	wave.WriteString("fmt ")
	_ = binary.Write(&wave, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wave, binary.LittleEndian, uint16(1))
	_ = binary.Write(&wave, binary.LittleEndian, channels)
	_ = binary.Write(&wave, binary.LittleEndian, sampleRate)
	_ = binary.Write(&wave, binary.LittleEndian, sampleRate*uint32(blockAlign))
	_ = binary.Write(&wave, binary.LittleEndian, blockAlign)
	_ = binary.Write(&wave, binary.LittleEndian, bitsPerSample)
	wave.WriteString("data")
	_ = binary.Write(&wave, binary.LittleEndian, declaredDataSize)
	wave.Write(make([]byte, samples*int(blockAlign)))
	return wave.Bytes()
}

func observeSpeechResponse(t *testing.T, contentType string, body []byte) Event {
	t.Helper()
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{Section: SectionVoice, DurationMS: 100}, contentType, io.NopCloser(bytes.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}
	return sink.events[0]
}

func TestResponseObserverMeasuresSpeechWaveDuration(t *testing.T) {
	recorded := observeSpeechResponse(t, "audio/wav", pcmWave(24000, 1, 16, 49365*2, 49365))

	if recorded.AudioSeconds < 2.0568 || recorded.AudioSeconds > 2.0569 {
		t.Fatalf("unexpected audio seconds %f", recorded.AudioSeconds)
	}
}

func TestResponseObserverMeasuresStreamedWaveWithoutDeclaredSize(t *testing.T) {
	recorded := observeSpeechResponse(t, "audio/wav", pcmWave(22050, 2, 16, 0xFFFFFFFF, 3*22050))

	if recorded.AudioSeconds != 3 {
		t.Fatalf("unexpected audio seconds %f", recorded.AudioSeconds)
	}
}

func TestResponseObserverMeasuresWaveLongerThanObservedPrefix(t *testing.T) {
	samples := observedBodyLimit
	recorded := observeSpeechResponse(t, "audio/wav", pcmWave(16000, 1, 16, uint32(samples*2), samples))

	if expected := float64(samples) / 16000; recorded.AudioSeconds != expected {
		t.Fatalf("unexpected audio seconds %f, expected %f", recorded.AudioSeconds, expected)
	}
}
