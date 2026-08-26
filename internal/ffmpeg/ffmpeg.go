// Package ffmpeg locates an ffmpeg binary and uses it for the two conversions
// the router cannot do natively: remuxing backend video output into MP4 for
// ComfyUI-compatible clients, and converting non-WAV audio into the WAV the
// whisper.cpp transcription path requires.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ErrNotAvailable is returned by every conversion when no working ffmpeg
// binary was located. Callers must turn this into an explicit client error
// rather than a truncated or unusable response.
var ErrNotAvailable = errors.New("ffmpeg is not available")

// Tool is a located, verified ffmpeg binary. The zero value is a valid
// "not available" tool: every method reports ErrNotAvailable.
type Tool struct {
	path string
}

// Available reports whether a working ffmpeg binary was located.
func (tool Tool) Available() bool {
	return tool.path != ""
}

// Path returns the resolved ffmpeg binary path, or "" when unavailable.
func (tool Tool) Path() string {
	return tool.path
}

// Locate resolves the ffmpeg binary from configuredPath, falling back to
// PATH when configuredPath is empty, and confirms it actually runs. It never
// returns an error for "ffmpeg is optional and missing" — callers that want
// that distinction check Available() on the zero Tool instead of treating a
// Locate error as fatal.
func Locate(configuredPath string) (Tool, error) {
	candidate := strings.TrimSpace(configuredPath)
	if candidate == "" {
		resolved, err := exec.LookPath("ffmpeg")
		if err != nil {
			return Tool{}, fmt.Errorf("ffmpeg not found on PATH: %w", err)
		}
		candidate = resolved
	}
	cmd := exec.Command(candidate, "-version")
	if err := cmd.Run(); err != nil {
		return Tool{}, fmt.Errorf("ffmpeg at %q did not run: %w", candidate, err)
	}
	return Tool{path: candidate}, nil
}

// RemuxToMP4 transcodes the video and audio streams of src (MJPG-AVI, WebM,
// or any container ffmpeg demuxes) into H.264/AAC MP4. Backend video output
// carries codecs (MJPG, VP8/VP9) that are not universally playable inside an
// MP4 box even when the container accepts them, so this re-encodes rather
// than stream-copies. Fragmented output (empty_moov+frag_keyframe) is
// required because dst is a non-seekable stream: a conventional faststart
// MP4 needs to rewrite the moov atom after the fact, which a pipe cannot do.
// The ftyp box a ComfyUI-style client checks for is still the first bytes.
func (tool Tool) RemuxToMP4(ctx context.Context, src io.Reader, dst io.Writer) error {
	if !tool.Available() {
		return ErrNotAvailable
	}
	return tool.run(ctx, src, dst, []string{
		"-i", "pipe:0",
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"pipe:1",
	})
}

// ConvertToWAV converts src (any ffmpeg-demuxable audio container) into the
// 16-bit mono PCM WAV the whisper.cpp transcription path requires.
func (tool Tool) ConvertToWAV(ctx context.Context, src io.Reader, dst io.Writer) error {
	if !tool.Available() {
		return ErrNotAvailable
	}
	return tool.run(ctx, src, dst, []string{
		"-i", "pipe:0",
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		"-f", "wav",
		"pipe:1",
	})
}

func (tool Tool) run(ctx context.Context, src io.Reader, dst io.Writer, args []string) error {
	cmd := exec.CommandContext(ctx, tool.path, args...)
	cmd.Stdin = src
	cmd.Stdout = dst
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
