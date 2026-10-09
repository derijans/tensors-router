package vllm

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type authorizedArchiveExtraction struct {
	ctx      context.Context
	root     *os.Root
	artifact Artifact
	kind     string
	seen     map[string]struct{}
	unpacked int64
	files    int
}

func extractAuthorizedArchive(ctx context.Context, archivePath string, destination string, artifact Artifact, artifactKind string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	archive, err := openRegularArchive(archivePath, artifactKind)
	if err != nil {
		return err
	}
	defer archive.Close()
	destinationRoot, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer destinationRoot.Close()
	source, err := decompressedArchive(archive, artifact.ArchiveFormat)
	if err != nil {
		return err
	}
	defer source.Close()
	extraction := authorizedArchiveExtraction{
		ctx:      ctx,
		root:     destinationRoot,
		artifact: artifact,
		kind:     artifactKind,
		seen:     make(map[string]struct{}),
	}
	if err := extraction.extractAll(tar.NewReader(source)); err != nil {
		return err
	}
	return extraction.verifyComplete()
}

func openRegularArchive(archivePath string, artifactKind string) (*os.File, error) {
	archiveInfo, err := os.Lstat(archivePath)
	if err != nil {
		return nil, err
	}
	if !archiveInfo.Mode().IsRegular() || archiveInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s archive is not a regular file", artifactKind)
	}
	return os.Open(archivePath)
}

func decompressedArchive(archive io.Reader, format string) (io.ReadCloser, error) {
	if format == "tar.gz" {
		return gzip.NewReader(archive)
	}
	return io.NopCloser(archive), nil
}

func (extraction *authorizedArchiveExtraction) extractAll(reader *tar.Reader) error {
	for {
		if err := extraction.ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := extraction.extractEntry(reader, header); err != nil {
			return err
		}
	}
}

func (extraction *authorizedArchiveExtraction) extractEntry(reader *tar.Reader, header *tar.Header) error {
	normalized, err := extraction.admitEntry(header.Name)
	if err != nil {
		return err
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return createArchiveDirectory(extraction.root, normalized)
	case tar.TypeReg:
		return extraction.extractRegularFile(reader, header.Size, normalized)
	default:
		return fmt.Errorf("%s archive contains unsupported entry %q", extraction.kind, normalized)
	}
}

func (extraction *authorizedArchiveExtraction) admitEntry(name string) (string, error) {
	normalized, err := normalizePortableArchivePath(name)
	if err != nil {
		return "", err
	}
	if _, exists := extraction.seen[normalized]; exists {
		return "", fmt.Errorf("%s archive contains duplicate path %q", extraction.kind, normalized)
	}
	extraction.seen[normalized] = struct{}{}
	extraction.files++
	if extraction.files > maximumSmokeModelFiles {
		return "", fmt.Errorf("%s archive contains too many entries", extraction.kind)
	}
	return normalized, nil
}

func (extraction *authorizedArchiveExtraction) extractRegularFile(reader io.Reader, size int64, normalized string) error {
	if size < 0 || size > extraction.artifact.UnpackedSize-extraction.unpacked {
		return fmt.Errorf("%s archive exceeds authorized unpacked size", extraction.kind)
	}
	written, err := writeArchiveRegularFile(extraction.ctx, extraction.root, normalized, reader, size)
	if err != nil {
		return err
	}
	if written != size {
		return fmt.Errorf("%s archive entry %q has invalid size", extraction.kind, normalized)
	}
	extraction.unpacked += written
	return nil
}

func (extraction *authorizedArchiveExtraction) verifyComplete() error {
	if extraction.unpacked != extraction.artifact.UnpackedSize || extraction.files == 0 {
		return fmt.Errorf("%s archive unpacked size %d does not match authorized size %d", extraction.kind, extraction.unpacked, extraction.artifact.UnpackedSize)
	}
	return nil
}

func writeArchiveRegularFile(ctx context.Context, root *os.Root, portablePath string, source io.Reader, size int64) (int64, error) {
	if err := createArchiveDirectory(root, pathDirectory(portablePath)); err != nil {
		return 0, err
	}
	file, err := root.OpenFile(filepath.FromSlash(portablePath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	written, copyError := copyContext(ctx, file, io.LimitReader(source, size+1))
	closeError := file.Close()
	if copyError != nil {
		return 0, copyError
	}
	if closeError != nil {
		return 0, closeError
	}
	return written, nil
}

func createArchiveDirectory(root *os.Root, portablePath string) error {
	if portablePath == "." {
		return nil
	}
	nativePath := filepath.FromSlash(portablePath)
	if err := root.MkdirAll(nativePath, 0o700); err != nil {
		return err
	}
	current := ""
	for _, component := range strings.Split(portablePath, "/") {
		if current == "" {
			current = component
		} else {
			current += string(filepath.Separator) + component
		}
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive directory %q is unsafe", portablePath)
		}
	}
	return nil
}

func pathDirectory(portablePath string) string {
	index := strings.LastIndexByte(portablePath, '/')
	if index < 0 {
		return "."
	}
	return portablePath[:index]
}

func normalizePortableArchivePath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, ":\x00") {
		return "", fmt.Errorf("archive path is invalid")
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || normalized != value && normalized+"/" != value || filepath.IsAbs(filepath.FromSlash(value)) {
		return "", fmt.Errorf("archive path %q is unsafe", value)
	}
	return normalized, nil
}
