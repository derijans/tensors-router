package vllm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const maximumManifestBytes = 4 << 20

type ManifestSource interface {
	Load(context.Context) (Manifest, string, error)
}

// ErrManifestNotPublished reports that a manifest source completed every integrity
// check it is responsible for and established that no manifest exists for this
// platform. It never reports a signature, freshness, transport, or digest failure.
var ErrManifestNotPublished = errors.New("vLLM runtime manifest is not published")

type ManifestTrust string

const (
	ManifestTrustTUF             ManifestTrust = "tuf"
	ManifestTrustOperatorPinned  ManifestTrust = "operator-pinned"
	ManifestTrustEmbeddedDefault ManifestTrust = "embedded-default"
	// ManifestTrustUnverified marks a manifest that authorizes nothing: it names a
	// package to install from PyPI with no digest pin. It is never produced by
	// ParseManifest, so no TUF-signed or operator-pinned manifest can ever carry it -
	// only UnverifiedManifestSource, which an operator must explicitly opt into.
	ManifestTrustUnverified ManifestTrust = "unverified"
	ManifestTrustUnknown    ManifestTrust = "unknown"
)

type ResolvingManifestSource interface {
	ManifestSource
	Resolve(context.Context) (Manifest, string, ManifestTrust, error)
}

func ResolveManifest(ctx context.Context, source ManifestSource) (Manifest, string, ManifestTrust, error) {
	if resolving, ok := source.(ResolvingManifestSource); ok {
		return resolving.Resolve(ctx)
	}
	manifest, digest, err := source.Load(ctx)
	return manifest, digest, staticManifestTrust(source), err
}

func staticManifestTrust(source ManifestSource) ManifestTrust {
	type trusted interface{ ManifestTrust() ManifestTrust }
	if value, ok := source.(trusted); ok {
		return value.ManifestTrust()
	}
	return ManifestTrustUnknown
}

// FallbackManifestSource tries Primary first and falls back to Fallback only when
// Primary reports ErrManifestNotPublished. Every other failure is returned unchanged,
// so a tampered, expired, or unreachable repository still fails closed.
type FallbackManifestSource struct {
	Primary  ManifestSource
	Fallback ManifestSource
}

func (source FallbackManifestSource) Load(ctx context.Context) (Manifest, string, error) {
	manifest, digest, _, err := source.Resolve(ctx)
	return manifest, digest, err
}

func (source FallbackManifestSource) Resolve(ctx context.Context) (Manifest, string, ManifestTrust, error) {
	if source.Primary == nil {
		if source.Fallback == nil {
			return Manifest{}, "", ManifestTrustUnknown, fmt.Errorf("no vLLM manifest source is configured")
		}
		return ResolveManifest(ctx, source.Fallback)
	}
	manifest, digest, trust, err := ResolveManifest(ctx, source.Primary)
	if err == nil {
		return manifest, digest, trust, nil
	}
	if !errors.Is(err, ErrManifestNotPublished) || source.Fallback == nil {
		return Manifest{}, "", ManifestTrustUnknown, err
	}
	fallbackManifest, fallbackDigest, fallbackTrust, fallbackErr := ResolveManifest(ctx, source.Fallback)
	if fallbackErr != nil {
		return Manifest{}, "", ManifestTrustUnknown, fmt.Errorf("%w; no fallback manifest is available: %v", err, fallbackErr)
	}
	return fallbackManifest, fallbackDigest, fallbackTrust, nil
}

type AuthorizedManifestFile struct {
	Path          string
	Authorization ArtifactAuthorization
}

func (source AuthorizedManifestFile) ManifestTrust() ManifestTrust {
	return ManifestTrustOperatorPinned
}

func (source AuthorizedManifestFile) Load(_ context.Context) (Manifest, string, error) {
	content, err := readBoundedRegularFile(source.Path, maximumManifestBytes)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("read authorized vLLM manifest: %w", err)
	}
	return ParseAuthorizedManifest(content, source.Authorization)
}

func ParseAuthorizedManifest(content []byte, authorization ArtifactAuthorization) (Manifest, string, error) {
	if authorization.Length <= 0 || authorization.Length != int64(len(content)) {
		return Manifest{}, "", fmt.Errorf("vLLM manifest length %d does not match authorized length %d", len(content), authorization.Length)
	}
	digest := sha256.Sum256(content)
	digestText := hex.EncodeToString(digest[:])
	if !equalSHA256(digestText, authorization.SHA256) {
		return Manifest{}, "", fmt.Errorf("vLLM manifest SHA-256 does not match TUF target metadata")
	}
	manifest, err := ParseManifest(content)
	if err != nil {
		return Manifest{}, "", err
	}
	return manifest, digestText, nil
}

func ParseManifest(content []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode vLLM manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Manifest{}, err
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("unsupported vLLM manifest schema version %d", manifest.SchemaVersion)
	}
	if err := validatePinnedVersion("release", manifest.Release); err != nil {
		return err
	}
	if len(manifest.Profiles) == 0 {
		return fmt.Errorf("vLLM manifest contains no profiles")
	}
	profileIDs := make(map[string]struct{}, len(manifest.Profiles))
	for index, profile := range manifest.Profiles {
		if err := validateProfile(profile); err != nil {
			return fmt.Errorf("vLLM profile %d: %w", index, err)
		}
		if _, exists := profileIDs[profile.ID]; exists {
			return fmt.Errorf("duplicate vLLM profile %q", profile.ID)
		}
		profileIDs[profile.ID] = struct{}{}
	}
	return nil
}

func validateArtifactRoles(installMethod string, roles map[string]int) error {
	allowed := map[string]map[string]bool{
		"wheel":  {"python": true, "uv": true, "vllm": true, "plugin": true, "dependency": true, "smoke_model": true},
		"source": {"python": true, "uv": true, "source": true, "plugin": true, "dependency": true, "smoke_model": true},
		"oci":    {"oci": true, "smoke_model": true},
	}[installMethod]
	for role := range roles {
		if !allowed[role] {
			return fmt.Errorf("%s profile cannot contain %s artifacts", installMethod, role)
		}
	}
	return nil
}

func validOCIImage(value string) bool {
	const prefix = "sha256:"
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, prefix) && validSHA256(strings.TrimPrefix(value, prefix))
}

func profileHasPrerequisite(profile Profile, id string) bool {
	for _, prerequisite := range profile.Prerequisites {
		if prerequisite.ID == id {
			return true
		}
	}
	return false
}

func incompatibleReason(profile Profile, detection Detection) string {
	if !containsFold(profile.OperatingSystems, detection.OS) {
		return "operating system " + detection.OS + " is unsupported"
	}
	if !containsFold(profile.Architectures, detection.Architecture) {
		return "architecture " + detection.Architecture + " is unsupported"
	}
	if !intersectsFold(profile.Devices, detection.Devices) {
		return "detected device is unsupported"
	}
	if missing := missingProfilePrerequisites(profile, detection); len(missing) > 0 {
		return "missing prerequisites: " + strings.Join(missing, ", ")
	}
	return ""
}

func missingProfilePrerequisites(profile Profile, detection Detection) []string {
	missing := make([]string, 0)
	for _, prerequisite := range profile.Prerequisites {
		if !detection.Prerequisites[prerequisite.ID] {
			missing = append(missing, prerequisite.Description)
		}
	}
	return missing
}

func detectedAccelerators(devices []string) []string {
	accelerators := make([]string, 0, len(devices))
	for _, device := range devices {
		if !strings.EqualFold(device, "cpu") {
			accelerators = append(accelerators, device)
		}
	}
	return accelerators
}

func readBoundedRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("%q size %d is outside allowed range", path, info.Size())
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
		return nil, fmt.Errorf("%q changed before reading", path)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	finishedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if len(content) != int(openedInfo.Size()) || finishedInfo.Size() != openedInfo.Size() || !finishedInfo.ModTime().Equal(openedInfo.ModTime()) {
		return nil, fmt.Errorf("%q changed while reading", path)
	}
	return content, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("vLLM manifest contains trailing JSON")
		}
		return fmt.Errorf("decode vLLM manifest trailing content: %w", err)
	}
	return nil
}

var exactStableVersionPattern = regexp.MustCompile(`^v?[0-9]+(?:\.[0-9]+)*(?:\.post[0-9]+)?(?:\+[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

func validatePinnedVersion(label string, value string) error {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if !exactStableVersionPattern.MatchString(value) || strings.Contains(lower, "latest") || strings.Contains(lower, "nightly") || strings.Contains(lower, "dev") {
		return fmt.Errorf("%s must be an exact stable version", label)
	}
	return nil
}

func validateSet(values []string, valid func(string) bool) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !valid(value) {
			return fmt.Errorf("unsupported value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate value %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validOS(value string) bool {
	return value == "linux" || value == "darwin"
}

func validArchitecture(value string) bool {
	return value == "amd64" || value == "arm64"
}

func validDevice(value string) bool {
	switch value {
	case "cpu", "cuda", "rocm", "xpu", "metal":
		return true
	default:
		return false
	}
}

func safeIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func safePackageName(value string) bool {
	return safeIdentifier(value)
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == sha256.Size
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func equalSHA256(left string, right string) bool {
	leftBytes, leftError := hex.DecodeString(strings.TrimSpace(left))
	rightBytes, rightError := hex.DecodeString(strings.TrimSpace(right))
	return leftError == nil && rightError == nil && len(leftBytes) == sha256.Size && bytes.Equal(leftBytes, rightBytes)
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func intersectsFold(left []string, right []string) bool {
	for _, value := range left {
		if containsFold(right, value) {
			return true
		}
	}
	return false
}
