package main

import (
	"fmt"

	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
)

func indexDownloadedArtifact(index *modelassets.Index, artifact downloader.ArtifactRecord) error {
	asset, err := downloadedArtifactAsset(index, artifact)
	if err != nil {
		return err
	}
	if asset.SHA256 != artifact.SHA256 {
		return fmt.Errorf("downloaded artifact hash differs from its record")
	}
	verificationSource := artifact.VerificationSource
	if verificationSource == "" {
		verificationSource = "sha256"
	}
	if err := index.SetVerificationSource(asset.SHA256, verificationSource); err != nil {
		return err
	}
	origin := modelassets.Origin{Repository: artifact.Repository, Commit: artifact.Revision, Path: artifact.RepositoryPath}
	if origin.URI() == "" {
		return nil
	}
	return index.BindOrigin(asset.SHA256, origin)
}

func downloadedArtifactAsset(index *modelassets.Index, artifact downloader.ArtifactRecord) (modelassets.Asset, error) {
	if artifact.VerificationSource != "sidecar" {
		asset, recorded, err := index.RecordVerifiedFile(artifact.Path, artifact.SHA256, artifact.Size, artifact.ModifiedUnixNano)
		if err != nil || recorded {
			return asset, err
		}
	}
	return index.IndexFile(artifact.Path)
}
