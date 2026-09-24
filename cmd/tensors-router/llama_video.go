package main

import (
	"os/exec"
	"path/filepath"

	"tensors-router/internal/ffmpeg"
)

func llamaVideoFFmpegDir(tool ffmpeg.Tool) string {
	if !tool.Available() {
		return ""
	}
	resolved, err := exec.LookPath(tool.Path())
	if err != nil {
		return ""
	}
	dir, err := filepath.Abs(filepath.Dir(resolved))
	if err != nil {
		return ""
	}
	if _, err := exec.LookPath(filepath.Join(dir, "ffprobe")); err != nil {
		return ""
	}
	return dir
}
