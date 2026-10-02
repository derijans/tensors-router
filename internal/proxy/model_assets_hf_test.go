package proxy

import (
	"context"
	"testing"
	"time"

	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
)

const interruptedAssetCommit = "0123456789abcdef0123456789abcdef01234567"

const interruptedAssetHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type interruptedAssetDownloader struct {
	downloader.Service
}

func (interruptedAssetDownloader) Repository(context.Context, downloader.RepositoryRequest) (downloader.RepositoryDetails, error) {
	return downloader.RepositoryDetails{Commit: interruptedAssetCommit, Files: []downloader.File{{Path: "model.gguf", LFSHash: interruptedAssetHash}}}, nil
}

func (interruptedAssetDownloader) CreateJob(context.Context, downloader.CreateJobRequest) (downloader.DownloadJob, error) {
	return downloader.DownloadJob{ID: "job", State: downloader.JobRunning}, nil
}

func (interruptedAssetDownloader) Job(string) (downloader.DownloadJob, bool, error) {
	return downloader.DownloadJob{ID: "job", State: downloader.JobRunning}, true, nil
}

func (interruptedAssetDownloader) Subscribe(string) (<-chan downloader.DownloadJob, func()) {
	events := make(chan downloader.DownloadJob)
	close(events)
	return events, func() {}
}

func TestHFAssetDownloadStopsWaitingWhenSubscriptionCloses(t *testing.T) {
	assets := &assetManager{downloader: interruptedAssetDownloader{}}
	reference := modelassets.Reference{Hash: interruptedAssetHash, Filename: "model.gguf"}
	origin := modelassets.Origin{Repository: "owner/model", Commit: interruptedAssetCommit, Path: "model.gguf"}
	finished := make(chan bool, 1)
	go func() {
		_, found := assets.downloadHFAsset(reference, origin)
		finished <- found
	}()
	select {
	case found := <-finished:
		if found {
			t.Fatal("an interrupted download was reported as resolved")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("asset resolution kept spinning on a closed subscription")
	}
}
