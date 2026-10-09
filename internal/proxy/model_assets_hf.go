package proxy

import (
	"context"
	"strings"
	"time"

	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
)

const hfDownloadPollInterval = 30 * time.Second

func (assets *assetManager) resolveAssetReferenceDetailed(reference modelassets.Reference) (modelassets.Resolution, bool) {
	if path, found := assets.findStoredAsset(reference); found {
		return modelassets.Resolution{Path: path, Source: "local", Verification: "sha256"}, true
	}
	if path, found := assets.resolvePeerAssetPath(reference.Hash, reference.Filename); found {
		return modelassets.Resolution{Path: path, Source: "peer", Verification: "sha256"}, true
	}
	return assets.resolveHFAsset(reference)
}

func (assets *assetManager) findStoredAsset(reference modelassets.Reference) (string, bool) {
	if path, found := assets.index.Find(reference.Hash, reference.Filename); found {
		return path, true
	}
	path, found, err := assets.index.FindInRoots(reference.Hash, reference.Filename, assets.fileRoots)
	return path, err == nil && found
}

func (assets *assetManager) resolveHFAsset(reference modelassets.Reference) (modelassets.Resolution, bool) {
	if reference.HF != "" {
		if origin, err := modelassets.ParseHFURI(reference.HF); err == nil {
			if resolution, found := assets.downloadHFResolution(reference, origin, "config_hf"); found {
				return resolution, true
			}
		}
	}
	if origin, found := assets.index.Origin(reference.Hash); found {
		if resolution, downloaded := assets.downloadHFResolution(reference, origin, "learned_hf"); downloaded {
			return resolution, true
		}
	}
	origin, found := assets.findUniqueExactHFOrigin(reference)
	if !found || assets.index.BindOrigin(reference.Hash, origin) != nil {
		return modelassets.Resolution{}, false
	}
	return assets.downloadHFResolution(reference, origin, "candidate_hf")
}

func (assets *assetManager) downloadHFResolution(reference modelassets.Reference, origin modelassets.Origin, source string) (modelassets.Resolution, bool) {
	path, downloaded := assets.downloadHFAsset(reference, origin)
	if !downloaded {
		return modelassets.Resolution{}, false
	}
	return modelassets.Resolution{Path: path, Source: source, Verification: "lfs_sha256", Commit: origin.Commit}, true
}

func (assets *assetManager) downloadHFAsset(reference modelassets.Reference, origin modelassets.Origin) (string, bool) {
	if assets.downloader == nil || origin.URI() == "" || reference.Hash == "" || reference.Filename == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelOperationTimeout)
	defer cancel()
	if !assets.originServesLFSHash(ctx, reference, origin) {
		return "", false
	}
	job, err := assets.downloader.CreateJob(ctx, downloader.CreateJobRequest{Repository: origin.Repository, Revision: origin.Commit, Files: []string{origin.Path}})
	if err != nil {
		return "", false
	}
	return assets.awaitHFDownload(ctx, job.ID, reference, origin)
}

func (assets *assetManager) originServesLFSHash(ctx context.Context, reference modelassets.Reference, origin modelassets.Origin) bool {
	details, err := assets.downloader.Repository(ctx, downloader.RepositoryRequest{Repository: origin.Repository, Revision: origin.Commit})
	if err != nil || details.Commit != origin.Commit {
		return false
	}
	for _, file := range details.Files {
		if file.Path == origin.Path && strings.EqualFold(file.LFSHash, reference.Hash) {
			return true
		}
	}
	return false
}

func (assets *assetManager) awaitHFDownload(ctx context.Context, jobID string, reference modelassets.Reference, origin modelassets.Origin) (string, bool) {
	events, unsubscribe := assets.downloader.Subscribe(jobID)
	defer unsubscribe()
	poll := time.NewTicker(hfDownloadPollInterval)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", false
		case event, open := <-events:
			if !open || downloadJobStopped(event.State) {
				return "", false
			}
			if event.State == downloader.JobCompleted {
				return assets.indexDownloadedHFArtifact(reference, origin)
			}
		case <-poll.C:
			current, found, err := assets.downloader.Job(jobID)
			if err != nil || !found || downloadJobStopped(current.State) {
				return "", false
			}
		}
	}
}

func downloadJobStopped(state downloader.JobState) bool {
	return state == downloader.JobFailed || state == downloader.JobCancelled
}

func (assets *assetManager) indexDownloadedHFArtifact(reference modelassets.Reference, origin modelassets.Origin) (string, bool) {
	artifacts, err := assets.downloader.Artifacts()
	if err != nil {
		return "", false
	}
	for _, artifact := range artifacts {
		if artifact.SHA256 == reference.Hash && artifact.Repository == origin.Repository && artifact.RepositoryPath == origin.Path && artifact.Revision == origin.Commit {
			return assets.adoptDownloadedArtifact(artifact.Path, reference, origin)
		}
	}
	return "", false
}

func (assets *assetManager) adoptDownloadedArtifact(path string, reference modelassets.Reference, origin modelassets.Origin) (string, bool) {
	asset, err := assets.index.IndexFile(path)
	if err != nil || asset.SHA256 != reference.Hash {
		return "", false
	}
	if err := assets.index.SetVerificationSource(reference.Hash, "hf_lfs_sha256"); err != nil {
		return "", false
	}
	if err := assets.index.BindOrigin(reference.Hash, origin); err != nil {
		return "", false
	}
	return assets.index.Find(reference.Hash, reference.Filename)
}
