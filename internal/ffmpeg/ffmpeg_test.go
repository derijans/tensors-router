package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestLocateFallsBackToPathAndVerifiesTheBinaryRuns(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	tool, err := Locate("")
	if err != nil {
		t.Fatalf("expected ffmpeg to be found on PATH: %v", err)
	}
	if !tool.Available() || tool.Path() == "" {
		t.Fatalf("located tool reports unavailable: %#v", tool)
	}
}

func TestLocateRejectsAMissingConfiguredPath(t *testing.T) {
	if _, err := Locate(`C:\definitely\not\a\real\ffmpeg.exe`); err == nil {
		t.Fatal("expected an error for a configured path that does not exist")
	}
}

func TestZeroToolReportsUnavailableAndRefusesConversions(t *testing.T) {
	var tool Tool
	if tool.Available() {
		t.Fatal("zero-value Tool must report unavailable")
	}
	if err := tool.RemuxToMP4(context.Background(), bytes.NewReader(nil), &bytes.Buffer{}); !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("expected ErrNotAvailable from RemuxToMP4, got %v", err)
	}
	if err := tool.ConvertToWAV(context.Background(), bytes.NewReader(nil), &bytes.Buffer{}); !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("expected ErrNotAvailable from ConvertToWAV, got %v", err)
	}
}

func TestRemuxToMP4ProducesAPlayableFtypBox(t *testing.T) {
	tool, err := Locate("")
	if err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	avi := generateTestAVI(t)
	var mp4 bytes.Buffer
	if err := tool.RemuxToMP4(context.Background(), bytes.NewReader(avi), &mp4); err != nil {
		t.Fatalf("RemuxToMP4 failed: %v", err)
	}
	out := mp4.Bytes()
	if len(out) < 12 {
		t.Fatalf("output too short to be MP4: %d bytes", len(out))
	}
	if string(out[4:8]) != "ftyp" {
		t.Fatalf("output is not a valid MP4 container: bytes[4:8] = %q", out[4:8])
	}
}

func TestConvertToWAVProducesARIFFWaveHeader(t *testing.T) {
	tool, err := Locate("")
	if err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	tone := generateTestTone(t)
	var wav bytes.Buffer
	if err := tool.ConvertToWAV(context.Background(), bytes.NewReader(tone), &wav); err != nil {
		t.Fatalf("ConvertToWAV failed: %v", err)
	}
	out := wav.Bytes()
	if len(out) < 12 || string(out[0:4]) != "RIFF" || string(out[8:12]) != "WAVE" {
		t.Fatalf("output is not a valid WAV file: %#v", out[:min(12, len(out))])
	}
}

// generateTestAVI synthesizes a tiny MJPG-AVI with a silent audio track using
// ffmpeg itself, mirroring what KoboldCpp's video_output_type=1 produces.
func generateTestAVI(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=8:duration=1",
		"-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono",
		"-shortest",
		"-c:v", "mjpeg", "-c:a", "pcm_s16le",
		"-f", "avi", "pipe:1",
	)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to synthesize test AVI: %v", err)
	}
	return buf.Bytes()
}

// generateTestTone synthesizes a tiny non-WAV (MP3) audio clip so
// ConvertToWAV has something real to transcode.
func generateTestTone(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:a", "libmp3lame", "-f", "mp3", "pipe:1",
	)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to synthesize test tone: %v", err)
	}
	return buf.Bytes()
}
