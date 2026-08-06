package loadcapture

import (
	"bytes"
	"testing"
	"time"
)

func TestHubPreservesStreamOrderAndBoundsSubscribers(t *testing.T) {
	hub := NewHub()
	full := hub.Subscribe(10)
	limited := hub.Subscribe(3)
	if _, err := hub.Stdout().Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Stderr().Write([]byte("cde")); err != nil {
		t.Fatal(err)
	}
	captured := full()
	if len(captured.Chunks) != 2 || captured.Chunks[0].Stream != StreamStdout || captured.Chunks[1].Stream != StreamStderr || captured.Chunks[0].Sequence >= captured.Chunks[1].Sequence || captured.Chunks[1].Offset < 0 {
		t.Fatalf("unexpected captured output: %#v", captured)
	}
	bounded := limited()
	if !bounded.Truncated || bounded.CapturedBytes != 3 {
		t.Fatalf("unexpected bounded capture: %#v", bounded)
	}
	if captured.Chunks[0].Offset > time.Second {
		t.Fatalf("unexpected capture offset: %s", captured.Chunks[0].Offset)
	}
}

func TestHubSplitsOversizedWritesWithoutLosingBytes(t *testing.T) {
	hub := NewHub()
	finish := hub.Subscribe(int64(maxChunkBytes * 2))
	payload := make([]byte, maxChunkBytes+1)
	for index := range payload {
		payload[index] = byte(index)
	}
	if _, err := hub.Stdout().Write(payload); err != nil {
		t.Fatal(err)
	}
	captured := finish()
	if len(captured.Chunks) != 2 || len(captured.Chunks[0].Payload) != maxChunkBytes || len(captured.Chunks[1].Payload) != 1 {
		t.Fatalf("unexpected chunk boundaries: %#v", captured.Chunks)
	}
	if captured.CapturedBytes != int64(len(payload)) || captured.Truncated {
		t.Fatalf("unexpected capture accounting: %#v", captured)
	}
	joined := append(append([]byte{}, captured.Chunks[0].Payload...), captured.Chunks[1].Payload...)
	if !bytes.Equal(joined, payload) {
		t.Fatal("captured bytes changed")
	}
}
