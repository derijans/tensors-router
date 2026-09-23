package proxy

import (
	"context"
	"strings"
	"time"

	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
)

func (assets *assetManager) resolveAssetReferenceDetailed(reference modelassets.Reference) (modelassets.Resolution, bool) {
	if path, found := assets.index.Find(reference.Hash, reference.Filename); found {
		return modelassets.Resolution{Path: path, Source: "local", Verification: "sha256"}, true
	}
	if path, found, err := assets.index.FindInRoots(reference.Hash, reference.Filename, assets.fileRoots); err == nil && found {
		return modelassets.Resolution{Path: path, Source: "local", Verification: "sha256"}, true
	}
	if path, found := assets.resolvePeerAssetPath(reference.Hash, reference.Filename); found {
		return modelassets.Resolution{Path: path, Source: "peer", Verification: "sha256"}, true
	}
	if reference.HF != "" {
		if origin, err := modelassets.ParseHFURI(reference.HF); err == nil {
			if path, found := assets.downloadHFAsset(reference, origin); found {
				return modelassets.Resolution{Path: path, Source: "config_hf", Verification: "lfs_sha256", Commit: origin.Commit}, true
			}
		}
	}
	if origin, found := assets.index.Origin(reference.Hash); found {
		if path, downloaded := assets.downloadHFAsset(reference, origin); downloaded {
			return modelassets.Resolution{Path: path, Source: "learned_hf", Verification: "lfs_sha256", Commit: origin.Commit}, true
		}
	}
	if origin, found := assets.findUniqueExactHFOrigin(reference); found {
		if err := assets.index.BindOrigin(reference.Hash, origin); err == nil {
			if path, downloaded := assets.downloadHFAsset(reference, origin); downloaded {
				return modelassets.Resolution{Path: path, Source: "candidate_hf", Verification: "lfs_sha256", Commit: origin.Commit}, true
			}
		}
	}
	return modelassets.Resolution{}, false
}

func (assets *assetManager) downloadHFAsset(reference modelassets.Reference, origin modelassets.Origin) (string, bool) {
	if assets.downloader == nil || origin.URI() == "" || reference.Hash == "" || reference.Filename == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelOperationTimeout)
	defer cancel()
	details, err := assets.downloader.Repository(ctx, downloader.RepositoryRequest{Repository: origin.Repository, Revision: origin.Commit})
	if err != nil || details.Commit != origin.Commit {
		return "", false
	}
	verified := false
	for _, file := range details.Files {
		if file.Path == origin.Path && strings.EqualFold(file.LFSHash, reference.Hash) {
			verified = true
			break
		}
	}
	if !verified {
		return "", false
	}
	job, err := assets.downloader.CreateJob(ctx, downloader.CreateJobRequest{Repository: origin.Repository, Revision: origin.Commit, Files: []string{origin.Path}})
	if err != nil {
		return "", false
	}
	events, unsubscribe := assets.downloader.Subscribe(job.ID)
	defer unsubscribe()
	poll := time.NewTicker(30 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", false
		case event := <-events:
			if event.State == downloader.JobFailed || event.State == downloader.JobCancelled {
				return "", false
			}
			if event.State != downloader.JobCompleted {
				continue
			}
			artifacts, err := assets.downloader.Artifacts()
			if err != nil {
				return "", false
			}
			for _, artifact := range artifacts {
				if artifact.SHA256 != reference.Hash || artifact.Repository != origin.Repository || artifact.RepositoryPath != origin.Path || artifact.Revision != origin.Commit {
					continue
				}
				asset, err := assets.index.IndexFile(artifact.Path)
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
			return "", false
		case <-poll.C:
			current, found, err := assets.downloader.Job(job.ID)
			if err != nil || !found || current.State == downloader.JobFailed || current.State == downloader.JobCancelled {
				return "", false
			}
		}
	}
}
