package tufpublish

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"tensors-router/internal/atomicfile"

	"github.com/sigstore/sigstore/pkg/signature"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
)

const (
	upstreamTargetsMetadataFile = "upstream-targets.json"
	upstreamTargetsRoleName     = "upstream-targets"
	snapshotMetadataFile        = "snapshot.json"
)

type publishedMetadata struct {
	root      *tufmetadata.Metadata[tufmetadata.RootType]
	top       *tufmetadata.Metadata[tufmetadata.TargetsType]
	topBytes  []byte
	delegated *tufmetadata.Metadata[tufmetadata.TargetsType]
	snapshot  *tufmetadata.Metadata[tufmetadata.SnapshotType]
	timestamp *tufmetadata.Metadata[tufmetadata.TimestampType]
}

type publicationSigners struct {
	upstream  signature.Signer
	snapshot  signature.Signer
	timestamp signature.Signer
}

type signedPublication struct {
	delegatedVersion int64
	delegated        []byte
	snapshotVersion  int64
	snapshot         []byte
	timestamp        []byte
}

func Publish(repository, output string, targetBodies map[string][]byte, secrets SigningSecrets, now time.Time) error {
	current, err := loadPublishedMetadata(filepath.Join(repository, "metadata"))
	if err != nil {
		return err
	}
	signers, err := authorizePublication(current, targetBodies, secrets, now)
	if err != nil {
		return err
	}
	publication, err := signPublication(current, targetBodies, signers, now)
	if err != nil {
		return err
	}
	if err := preparePublicationOutput(repository, output); err != nil {
		return err
	}
	if err := writePublication(output, publication, targetBodies); err != nil {
		return err
	}
	expectedTargets := make([]string, 0, len(targetBodies))
	for targetPath := range targetBodies {
		expectedTargets = append(expectedTargets, targetPath)
	}
	return Verify(output, expectedTargets)
}

func loadPublishedMetadata(metadataDir string) (publishedMetadata, error) {
	var current publishedMetadata
	var err error
	if current.root, _, err = readMetadata(filepath.Join(metadataDir, "root.json"), maxTrustedRootBytes, tufmetadata.Root().FromBytes); err != nil {
		return current, err
	}
	if current.top, current.topBytes, err = readMetadata(filepath.Join(metadataDir, "targets.json"), maxTargetsMetadataBytes, tufmetadata.Targets().FromBytes); err != nil {
		return current, err
	}
	if current.delegated, _, err = readMetadata(filepath.Join(metadataDir, upstreamTargetsMetadataFile), maxTargetsMetadataBytes, tufmetadata.Targets().FromBytes); err != nil {
		return current, err
	}
	if current.snapshot, _, err = readMetadata(filepath.Join(metadataDir, snapshotMetadataFile), maxSnapshotMetadataBytes, tufmetadata.Snapshot().FromBytes); err != nil {
		return current, err
	}
	current.timestamp, _, err = readMetadata(filepath.Join(metadataDir, "timestamp.json"), maxTimestampMetadataBytes, tufmetadata.Timestamp().FromBytes)
	return current, err
}

func readMetadata[T any](path string, maximum int64, parse func([]byte) (T, error)) (T, []byte, error) {
	var empty T
	body, err := readBoundedFile(path, maximum)
	if err != nil {
		return empty, nil, err
	}
	parsed, err := parse(body)
	if err != nil {
		return empty, nil, err
	}
	return parsed, body, nil
}

func authorizePublication(current publishedMetadata, targetBodies map[string][]byte, secrets SigningSecrets, now time.Time) (publicationSigners, error) {
	if current.root.Signed.IsExpired(now) {
		return publicationSigners{}, fmt.Errorf("root metadata is expired")
	}
	upstreamRole, err := delegatedRole(current.top, upstreamTargetsRoleName)
	if err != nil {
		return publicationSigners{}, err
	}
	for targetPath := range targetBodies {
		allowed, err := upstreamRole.IsDelegatedPath(targetPath)
		if err != nil {
			return publicationSigners{}, fmt.Errorf("validate delegated target path %q: %w", targetPath, err)
		}
		if !allowed {
			return publicationSigners{}, fmt.Errorf("trusted targets do not delegate %q to upstream-targets", targetPath)
		}
	}
	var signers publicationSigners
	if signers.upstream, err = authorizedSigner(secrets.Upstream, upstreamRole.KeyIDs); err != nil {
		return publicationSigners{}, fmt.Errorf("upstream-targets key: %w", err)
	}
	if signers.snapshot, err = authorizedSigner(secrets.Snapshot, current.root.Signed.Roles["snapshot"].KeyIDs); err != nil {
		return publicationSigners{}, fmt.Errorf("snapshot key: %w", err)
	}
	if signers.timestamp, err = authorizedSigner(secrets.Timestamp, current.root.Signed.Roles["timestamp"].KeyIDs); err != nil {
		return publicationSigners{}, fmt.Errorf("timestamp key: %w", err)
	}
	return signers, nil
}

func signPublication(current publishedMetadata, targetBodies map[string][]byte, signers publicationSigners, now time.Time) (signedPublication, error) {
	delegated := tufmetadata.Targets(now.AddDate(0, 1, 0))
	delegated.Signed.Version = current.delegated.Signed.Version + 1
	for targetPath, body := range targetBodies {
		info, err := tufmetadata.TargetFile().FromBytes(targetPath, body, "sha256")
		if err != nil {
			return signedPublication{}, err
		}
		delegated.Signed.Targets[targetPath] = info
	}
	delegatedBytes, err := signedBytes(delegated, signers.upstream)
	if err != nil {
		return signedPublication{}, err
	}

	snapshot := tufmetadata.Snapshot(now.AddDate(0, 0, 14))
	snapshot.Signed.Version = current.snapshot.Signed.Version + 1
	snapshot.Signed.Meta["targets.json"] = publicationMeta(current.top.Signed.Version, current.topBytes)
	snapshot.Signed.Meta[upstreamTargetsMetadataFile] = publicationMeta(delegated.Signed.Version, delegatedBytes)
	snapshotBytes, err := signedBytes(snapshot, signers.snapshot)
	if err != nil {
		return signedPublication{}, err
	}

	timestamp := tufmetadata.Timestamp(now.AddDate(0, 0, 2))
	timestamp.Signed.Version = current.timestamp.Signed.Version + 1
	timestamp.Signed.Meta[snapshotMetadataFile] = publicationMeta(snapshot.Signed.Version, snapshotBytes)
	timestampBytes, err := signedBytes(timestamp, signers.timestamp)
	if err != nil {
		return signedPublication{}, err
	}
	return signedPublication{
		delegatedVersion: delegated.Signed.Version,
		delegated:        delegatedBytes,
		snapshotVersion:  snapshot.Signed.Version,
		snapshot:         snapshotBytes,
		timestamp:        timestampBytes,
	}, nil
}

func signedBytes[T tufmetadata.Roles](metadata *tufmetadata.Metadata[T], signer signature.Signer) ([]byte, error) {
	if _, err := metadata.Sign(signer); err != nil {
		return nil, err
	}
	return metadata.ToBytes(true)
}

func writePublication(output string, publication signedPublication, targetBodies map[string][]byte) error {
	metadataOutputs := map[string][]byte{
		fmt.Sprintf("%d.upstream-targets.json", publication.delegatedVersion): publication.delegated,
		upstreamTargetsMetadataFile:                                  publication.delegated,
		fmt.Sprintf("%d.snapshot.json", publication.snapshotVersion): publication.snapshot,
		snapshotMetadataFile:                                         publication.snapshot,
	}
	for name, body := range metadataOutputs {
		if err := atomicfile.Write(filepath.Join(output, "metadata", name), body, 0o644); err != nil {
			return err
		}
	}
	for targetPath, body := range targetBodies {
		digest := sha256.Sum256(body)
		directory, name := filepath.Split(filepath.FromSlash(targetPath))
		name = hex.EncodeToString(digest[:]) + "." + name
		if err := atomicfile.Write(filepath.Join(output, "targets", directory, name), body, 0o644); err != nil {
			return err
		}
	}
	return atomicfile.Write(filepath.Join(output, "metadata", "timestamp.json"), publication.timestamp, 0o644)
}
