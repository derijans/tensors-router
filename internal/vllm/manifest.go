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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// ManifestTrust names how strongly the manifest that was loaded is authorized.
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

// ResolvingManifestSource is implemented by sources that decide their trust tier while
// loading rather than statically.
type ResolvingManifestSource interface {
	ManifestSource
	Resolve(context.Context) (Manifest, string, ManifestTrust, error)
}

// ResolveManifest loads a manifest and reports which trust tier produced it.
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

func validateProfile(profile Profile) error {
	if !safeIdentifier(profile.ID) {
		return fmt.Errorf("invalid profile id %q", profile.ID)
	}
	if err := validatePinnedVersion("vLLM version", profile.VLLMVersion); err != nil {
		return err
	}
	if err := validatePinnedVersion("Python version", profile.PythonVersion); err != nil {
		return err
	}
	for plugin, version := range profile.PluginVersions {
		if !safePackageName(plugin) {
			return fmt.Errorf("invalid plugin name %q", plugin)
		}
		if err := validatePinnedVersion("plugin "+plugin+" version", version); err != nil {
			return err
		}
	}
	switch profile.InstallMethod {
	case "wheel", "source", "oci":
	default:
		return fmt.Errorf("unsupported installation method %q", profile.InstallMethod)
	}
	if len(profile.OperatingSystems) == 0 || len(profile.Architectures) == 0 || len(profile.Devices) == 0 {
		return fmt.Errorf("operating_systems, architectures, and devices are required")
	}
	if err := validateSet(profile.OperatingSystems, validOS); err != nil {
		return fmt.Errorf("operating systems: %w", err)
	}
	if err := validateSet(profile.Architectures, validArchitecture); err != nil {
		return fmt.Errorf("architectures: %w", err)
	}
	if err := validateSet(profile.Devices, validDevice); err != nil {
		return fmt.Errorf("devices: %w", err)
	}
	prerequisites := make(map[string]struct{}, len(profile.Prerequisites))
	for _, prerequisite := range profile.Prerequisites {
		if !safeIdentifier(prerequisite.ID) || strings.TrimSpace(prerequisite.Description) == "" {
			return fmt.Errorf("invalid prerequisite %q", prerequisite.ID)
		}
		if !supportedPrerequisite(prerequisite.ID) {
			return fmt.Errorf("unsupported prerequisite %q", prerequisite.ID)
		}
		if _, exists := prerequisites[prerequisite.ID]; exists {
			return fmt.Errorf("duplicate prerequisite %q", prerequisite.ID)
		}
		prerequisites[prerequisite.ID] = struct{}{}
	}
	if len(profile.Artifacts) == 0 {
		return fmt.Errorf("artifacts are required")
	}
	artifactNames := make(map[string]struct{}, len(profile.Artifacts))
	artifactRoles := make(map[string]int)
	for _, artifact := range profile.Artifacts {
		if err := validateArtifact(artifact); err != nil {
			return err
		}
		if _, exists := artifactNames[artifact.Name]; exists {
			return fmt.Errorf("duplicate artifact %q", artifact.Name)
		}
		artifactNames[artifact.Name] = struct{}{}
		artifactRoles[artifact.Role]++
	}
	if err := validateArtifactRoles(profile.InstallMethod, artifactRoles); err != nil {
		return err
	}
	if artifactRoles["smoke_model"] != 1 {
		return fmt.Errorf("%s profile requires exactly one signed smoke-model artifact", profile.InstallMethod)
	}
	if profile.InstallMethod == "oci" {
		if artifactRoles["oci"] != 1 {
			return fmt.Errorf("OCI profile requires exactly one OCI artifact")
		}
		if !validOCIImage(profile.OCIImage) {
			return fmt.Errorf("OCI profile requires an immutable sha256 image id")
		}
		if !profileHasPrerequisite(profile, "container_engine") {
			return fmt.Errorf("OCI profile requires container_engine prerequisite")
		}
		return nil
	}
	if profile.OCIImage != "" {
		return fmt.Errorf("oci_image is only valid for OCI profiles")
	}
	if artifactRoles["uv"] > 1 || artifactRoles["python"] != 1 {
		return fmt.Errorf("%s profile requires exactly one Python artifact and at most one uv fallback", profile.InstallMethod)
	}
	if artifactRoles["plugin"] != len(profile.PluginVersions) {
		return fmt.Errorf("%s profile must provide one plugin artifact for every pinned plugin version", profile.InstallMethod)
	}
	if profile.InstallMethod == "wheel" && artifactRoles["vllm"] != 1 {
		return fmt.Errorf("wheel profile requires exactly one vLLM artifact")
	}
	if profile.InstallMethod == "source" && artifactRoles["source"] != 1 {
		return fmt.Errorf("source profile requires exactly one source artifact")
	}
	if profile.InstallMethod == "source" && !profileHasPrerequisite(profile, "compiler") {
		return fmt.Errorf("source profile requires compiler prerequisite")
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

func validateArtifact(artifact Artifact) error {
	if artifact.Name == "" || artifact.Name != filepath.Base(artifact.Name) || strings.ContainsAny(artifact.Name, `/\`) {
		return fmt.Errorf("invalid artifact name %q", artifact.Name)
	}
	parsed, err := url.Parse(artifact.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("artifact %q must use an absolute credential-free HTTPS URL", artifact.Name)
	}
	if artifact.Size <= 0 {
		return fmt.Errorf("artifact %q has invalid size %d", artifact.Name, artifact.Size)
	}
	if !validSHA256(artifact.SHA256) {
		return fmt.Errorf("artifact %q has invalid SHA-256", artifact.Name)
	}
	switch artifact.Role {
	case "python", "uv", "vllm", "plugin", "source", "oci", "dependency", "smoke_model":
	default:
		return fmt.Errorf("artifact %q has unsupported role %q", artifact.Name, artifact.Role)
	}
	if artifact.Role == "smoke_model" || artifact.Role == "python" {
		if artifact.ArchiveFormat != "tar" && artifact.ArchiveFormat != "tar.gz" {
			return fmt.Errorf("artifact %q requires archive_format tar or tar.gz", artifact.Name)
		}
		if artifact.UnpackedSize <= 0 {
			return fmt.Errorf("artifact %q requires positive unpacked_size", artifact.Name)
		}
	} else if artifact.ArchiveFormat != "" || artifact.UnpackedSize != 0 || artifact.ExecutablePath != "" {
		return fmt.Errorf("artifact %q cannot contain archive metadata", artifact.Name)
	}
	if artifact.Role == "python" {
		if _, err := normalizePortableArchivePath(artifact.ExecutablePath); err != nil {
			return fmt.Errorf("Python artifact %q executable_path: %w", artifact.Name, err)
		}
	} else if artifact.ExecutablePath != "" {
		return fmt.Errorf("artifact %q executable_path is only valid for Python", artifact.Name)
	}
	return nil
}

func SelectProfile(manifest Manifest, requested string, detection Detection) (Profile, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "auto"
	}
	if requested != "auto" {
		for _, profile := range manifest.Profiles {
			if profile.ID != requested {
				continue
			}
			if reason := incompatibleReason(profile, detection); reason != "" {
				return Profile{}, fmt.Errorf("vLLM profile %q is not compatible: %s", requested, reason)
			}
			return profile, nil
		}
		return Profile{}, fmt.Errorf("vLLM profile %q is not present in authorized manifest", requested)
	}

	accelerators := detectedAccelerators(detection.Devices)
	candidates := make([]Profile, 0, len(manifest.Profiles))
	missingPrerequisites := make([]string, 0)
	for _, profile := range manifest.Profiles {
		if !containsFold(profile.OperatingSystems, detection.OS) || !containsFold(profile.Architectures, detection.Architecture) {
			continue
		}
		if len(accelerators) > 0 && !intersectsFold(profile.Devices, accelerators) {
			continue
		}
		if len(accelerators) == 0 && !containsFold(profile.Devices, "cpu") {
			continue
		}
		if missing := missingProfilePrerequisites(profile, detection); len(missing) > 0 {
			missingPrerequisites = append(missingPrerequisites, profile.ID+": "+strings.Join(missing, ", "))
			continue
		}
		candidates = append(candidates, profile)
	}
	if len(candidates) == 0 {
		if len(accelerators) > 0 && len(missingPrerequisites) > 0 {
			sort.Strings(missingPrerequisites)
			return Profile{}, fmt.Errorf("detected accelerator prerequisites are missing (%s); CPU fallback is disabled", strings.Join(missingPrerequisites, "; "))
		}
		return Profile{}, fmt.Errorf("no authorized vLLM profile supports %s/%s devices %s", detection.OS, detection.Architecture, strings.Join(detection.Devices, ","))
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].Priority == candidates[right].Priority {
			return candidates[left].ID < candidates[right].ID
		}
		return candidates[left].Priority > candidates[right].Priority
	})
	return candidates[0], nil
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
