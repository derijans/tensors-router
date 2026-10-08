package modelassets

import (
	"fmt"
	"path/filepath"
	"time"
)

func (index *Index) RecordVerifiedFile(path string, hash string, size int64, modTimeNano int64) (Asset, bool, error) {
	if !validHash(hash) {
		return Asset{}, false, fmt.Errorf("verified file hash is invalid")
	}
	absolute, info, err := regularFile(path)
	if err != nil {
		return Asset{}, false, err
	}
	if info.Size() != size || info.ModTime().UnixNano() != modTimeNano {
		return Asset{}, false, nil
	}
	filename := filepath.Base(absolute)
	if !safeFilename(filename) {
		return Asset{}, false, fmt.Errorf("asset has an unsafe filename")
	}
	asset := Asset{SHA256: hash, Filename: filename, Size: size, Path: absolute, VerificationSource: "sha256", VerifiedAt: time.Now().UTC()}
	index.mu.Lock()
	index.paths[absolute] = cachedPath{Size: size, ModTimeNano: modTimeNano, SHA256: hash}
	index.assets[hash] = asset
	index.mu.Unlock()
	return asset, true, index.persistAsset(asset)
}
