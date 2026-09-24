package main

import (
	"os/exec"
	"path/filepath"
	"testing"

	"tensors-router/internal/ffmpeg"
)

func TestLlamaVideoFFmpegDirIsEmptyWithoutFFmpeg(t *testing.T) {
	if dir := llamaVideoFFmpegDir(ffmpeg.Tool{}); dir != "" {
		t.Fatalf("expected no --video-ffmpeg-dir without a located ffmpeg, got %q", dir)
	}
}

func TestLlamaVideoFFmpegDirPointsAtFFmpegAndFFprobe(t *testing.T) {
	tool, err := ffmpeg.Locate("")
	if err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	dir := llamaVideoFFmpegDir(tool)
	_, probeErr := exec.LookPath(filepath.Join(filepath.Dir(tool.Path()), "ffprobe"))
	if probeErr != nil {
		if dir != "" {
			t.Fatalf("llama-server needs ffprobe beside ffmpeg; expected no directory, got %q", dir)
		}
		return
	}
	if dir == "" || !filepath.IsAbs(dir) {
		t.Fatalf("expected an absolute directory holding ffmpeg and ffprobe, got %q", dir)
	}
	if _, err := exec.LookPath(filepath.Join(dir, "ffmpeg")); err != nil {
		t.Fatalf("directory %q does not contain ffmpeg: %v", dir, err)
	}
}
