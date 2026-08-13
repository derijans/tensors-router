package tufpublish

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"

	"tensors-router/internal/atomicfile"
)

const vllmDelegatedPath = "runtimes/vllm/*"

type TargetsRotationRequest struct {
	SchemaVersion          int      `json:"schema_version"`
	Role                   string   `json:"role"`
	SourceVersion          int64    `json:"source_version"`
	SourceSHA256           string   `json:"source_sha256"`
	CandidateVersion       int64    `json:"candidate_version"`
	CandidateExpires       string   `json:"candidate_expires"`
	CanonicalPayloadSHA256 string   `json:"canonical_payload_sha256"`
	CanonicalPayloadFile   string   `json:"canonical_payload_file"`
	UnsignedMetadataFile   string   `json:"unsigned_metadata_file"`
	AuthorizedKeyIDs       []string `json:"authorized_key_ids"`
	RequiredSignatures     int      `json:"required_signatures"`
	AddedDelegatedPaths    []string `json:"added_delegated_paths"`
}

func PrepareTargetsRotation(repository, output string, expires, now time.Time) error {
	root, current, currentBytes, err := loadRotationSource(repository, now)
	if err != nil {
		return err
	}
	candidate, err := buildTargetsRotation(current, expires, now)
	if err != nil {
		return err
	}
	payload, err := cjson.EncodeCanonical(candidate.Signed)
	if err != nil {
		return fmt.Errorf("encode canonical targets signing payload: %w", err)
	}
	unsignedMetadata, err := cjson.EncodeCanonical(candidate)
	if err != nil {
		return fmt.Errorf("encode unsigned targets metadata: %w", err)
	}
	targetsRole := root.Signed.Roles[tufmetadata.TARGETS]
	keyIDs := slices.Clone(targetsRole.KeyIDs)
	sort.Strings(keyIDs)
	sourceDigest := sha256.Sum256(currentBytes)
	payloadDigest := sha256.Sum256(payload)
	request := TargetsRotationRequest{
		SchemaVersion:          1,
		Role:                   tufmetadata.TARGETS,
		SourceVersion:          current.Signed.Version,
		SourceSHA256:           hex.EncodeToString(sourceDigest[:]),
		CandidateVersion:       candidate.Signed.Version,
		CandidateExpires:       candidate.Signed.Expires.Format(time.RFC3339),
		CanonicalPayloadSHA256: hex.EncodeToString(payloadDigest[:]),
		CanonicalPayloadFile:   "targets.payload.json",
		UnsignedMetadataFile:   "targets.unsigned.json",
		AuthorizedKeyIDs:       keyIDs,
		RequiredSignatures:     targetsRole.Threshold,
		AddedDelegatedPaths:    []string{vllmDelegatedPath},
	}
	requestBytes, err := cjson.EncodeCanonical(request)
	if err != nil {
		return fmt.Errorf("encode canonical rotation request: %w", err)
	}
	if err := requireEmptyDirectory(output); err != nil {
		return err
	}
	for name, body := range map[string][]byte{
		"rotation-request.json": requestBytes,
		"targets.payload.json":  payload,
		"targets.unsigned.json": unsignedMetadata,
	} {
		if err := atomicfile.Write(filepath.Join(output, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func InstallTargetsRotation(repository, signedTargetsPath, output string, now time.Time) error {
	contained, err := pathIsWithin(repository, output)
	if err != nil {
		return err
	}
	if contained {
		return fmt.Errorf("rotation output cannot be inside the source repository")
	}
	root, current, _, err := loadRotationSource(repository, now)
	if err != nil {
		return err
	}
	signedTargetsBytes, err := readBoundedFile(signedTargetsPath, maxTargetsMetadataBytes)
	if err != nil {
		return fmt.Errorf("read offline-signed targets metadata: %w", err)
	}
	candidate, err := tufmetadata.Targets().FromBytes(signedTargetsBytes)
	if err != nil {
		return fmt.Errorf("parse offline-signed targets metadata: %w", err)
	}
	if err := root.VerifyDelegate(tufmetadata.TARGETS, candidate); err != nil {
		return fmt.Errorf("verify offline targets signature threshold: %w", err)
	}
	expected, err := buildTargetsRotation(current, candidate.Signed.Expires, now)
	if err != nil {
		return err
	}
	actualPayload, err := cjson.EncodeCanonical(candidate.Signed)
	if err != nil {
		return fmt.Errorf("encode signed targets payload: %w", err)
	}
	expectedPayload, err := cjson.EncodeCanonical(expected.Signed)
	if err != nil {
		return fmt.Errorf("encode expected targets payload: %w", err)
	}
	if !slices.Equal(actualPayload, expectedPayload) {
		return fmt.Errorf("offline-signed targets metadata contains changes outside the approved vLLM delegation rotation")
	}
	versionedName := fmt.Sprintf("%d.targets.json", candidate.Signed.Version)
	if _, err := os.Stat(filepath.Join(repository, "metadata", versionedName)); err == nil {
		return fmt.Errorf("targets metadata version %d already exists", candidate.Signed.Version)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := requireEmptyDirectory(output); err != nil {
		return err
	}
	if err := copyRepository(repository, output); err != nil {
		return err
	}
	signedBytes, err := candidate.ToBytes(true)
	if err != nil {
		return fmt.Errorf("encode verified targets metadata: %w", err)
	}
	for _, name := range []string{versionedName, "targets.json"} {
		if err := atomicfile.Write(filepath.Join(output, "metadata", name), signedBytes, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func loadRotationSource(repository string, now time.Time) (*tufmetadata.Metadata[tufmetadata.RootType], *tufmetadata.Metadata[tufmetadata.TargetsType], []byte, error) {
	metadataDirectory := filepath.Join(repository, "metadata")
	rootBytes, err := readBoundedFile(filepath.Join(metadataDirectory, "root.json"), maxTrustedRootBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	root, err := tufmetadata.Root().FromBytes(rootBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	if root.Signed.IsExpired(now) {
		return nil, nil, nil, fmt.Errorf("root metadata is expired")
	}
	if err := root.VerifyDelegate(tufmetadata.ROOT, root); err != nil {
		return nil, nil, nil, fmt.Errorf("verify root signature threshold: %w", err)
	}
	targetsRole, exists := root.Signed.Roles[tufmetadata.TARGETS]
	if !exists || targetsRole.Threshold < 2 {
		return nil, nil, nil, fmt.Errorf("top-level targets must require at least two offline signatures")
	}
	currentPath := filepath.Join(metadataDirectory, "targets.json")
	currentBytes, err := readBoundedFile(currentPath, maxTargetsMetadataBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	current, err := tufmetadata.Targets().FromBytes(currentBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := root.VerifyDelegate(tufmetadata.TARGETS, current); err != nil {
		return nil, nil, nil, fmt.Errorf("verify current targets signature threshold: %w", err)
	}
	if err := Verify(repository, nil); err != nil {
		return nil, nil, nil, fmt.Errorf("verify current TUF repository: %w", err)
	}
	return root, current, currentBytes, nil
}

func buildTargetsRotation(current *tufmetadata.Metadata[tufmetadata.TargetsType], expires, now time.Time) (*tufmetadata.Metadata[tufmetadata.TargetsType], error) {
	if current.Signed.Delegations == nil {
		return nil, fmt.Errorf("current targets metadata has no delegations")
	}
	if current.Signed.Version < 1 {
		return nil, fmt.Errorf("current targets version is invalid")
	}
	expires = expires.UTC().Truncate(time.Second)
	if !expires.After(now.UTC()) {
		return nil, fmt.Errorf("rotated targets expiration must be in the future")
	}
	if !expires.After(current.Signed.Expires) {
		return nil, fmt.Errorf("rotated targets expiration must advance beyond the current expiration")
	}
	if expires.After(now.UTC().AddDate(1, 0, 0)) {
		return nil, fmt.Errorf("rotated targets expiration cannot exceed one year")
	}
	currentBytes, err := current.ToBytes(false)
	if err != nil {
		return nil, err
	}
	candidate, err := tufmetadata.Targets().FromBytes(currentBytes)
	if err != nil {
		return nil, err
	}
	candidate.ClearSignatures()
	candidate.Signed.Version = current.Signed.Version + 1
	candidate.Signed.Expires = expires
	found := false
	for index := range candidate.Signed.Delegations.Roles {
		role := &candidate.Signed.Delegations.Roles[index]
		if role.Name != "upstream-targets" {
			continue
		}
		found = true
		if slices.Contains(role.Paths, vllmDelegatedPath) {
			return nil, fmt.Errorf("current targets already delegate %s", vllmDelegatedPath)
		}
		if len(role.PathHashPrefixes) != 0 || !slices.Equal(role.Paths, []string{"upstreams/*/*"}) {
			return nil, fmt.Errorf("upstream-targets must authorize only upstreams/*/* before rotation")
		}
		role.Paths = append(role.Paths, vllmDelegatedPath)
	}
	if !found {
		return nil, fmt.Errorf("current targets do not authorize upstream-targets")
	}
	return candidate, nil
}

func requireEmptyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output directory cannot be a symbolic link")
	}
	if err == nil && !info.IsDir() {
		return fmt.Errorf("output path must be a directory")
	}
	entries, err := os.ReadDir(path)
	if err == nil && len(entries) != 0 {
		return fmt.Errorf("output directory must be empty")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func pathIsWithin(parent, child string) (bool, error) {
	parentPath, err := filepath.Abs(parent)
	if err != nil {
		return false, err
	}
	childPath, err := filepath.Abs(child)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(parentPath, childPath)
	if err != nil {
		return false, err
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

func copyRepository(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository contains symbolic link %s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("repository contains non-regular file %s", path)
		}
		body, err := readBoundedFile(path, maxTargetsMetadataBytes)
		if err != nil {
			return err
		}
		return atomicfile.Write(target, body, 0o644)
	})
}
