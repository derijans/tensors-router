package main

import (
	"context"

	"tensors-router/internal/downloader"
	"tensors-router/internal/ffmpeg"
)

func (runtime *serveRuntime) startCompanions(ctx context.Context) {
	logger := runtime.startupLogger
	runtime.downloader, runtime.downloaderCapability = optionalDownloader(runtime.configPath, runtime.cfg.Downloader, logger)
	runtime.vllm, runtime.vllmUnavailableReason = optionalVLLMCompanion(ctx, runtime.configPath, runtime.cfg.VLLM, logger)
	if runtime.vllmUnavailableReason != "" {
		logger.Printf("vLLM companion unavailable reason=%q", runtime.vllmUnavailableReason)
	}
	if runtime.downloader != nil {
		runtime.indexDownloadedArtifacts()
	}
}

func (runtime *serveRuntime) indexDownloadedArtifacts() {
	assetIndex := runtime.assetIndex
	runtime.downloader.SetArtifactHandler(func(artifact downloader.ArtifactRecord) error {
		return indexDownloadedArtifact(assetIndex, artifact)
	})
	artifacts, err := runtime.downloader.Artifacts()
	if err != nil {
		runtime.startupLogger.Printf("download artifact listing failed; downloaded models are indexed as the downloader reports them error=%q", err)
	}
	for _, artifact := range artifacts {
		if indexErr := indexDownloadedArtifact(assetIndex, artifact); indexErr != nil {
			runtime.startupLogger.Printf("download artifact indexing failed error_type=%T", indexErr)
		}
	}
}

func (runtime *serveRuntime) locateFFmpeg() {
	tool, err := ffmpeg.Locate(runtime.cfg.FFmpeg.BinaryPath)
	if err != nil {
		runtime.logger.Printf("ffmpeg not available, video remuxing and non-WAV audio conversion are disabled: %v", err)
	} else {
		runtime.logger.Printf("ffmpeg located at %s", tool.Path())
	}
	runtime.ffmpeg = tool
}
