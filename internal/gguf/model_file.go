package gguf

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const maxSplitShards = 256

var splitShardPattern = regexp.MustCompile(`^(.*)-([0-9]{5})-of-([0-9]{5})\.gguf$`)

type ModelFile struct {
	Metadata  Metadata
	FileBytes int64
}

func ReadModelFile(path string) (ModelFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return ModelFile{}, err
	}
	defer file.Close()
	metadata, err := ReadMetadata(file, DefaultLimits())
	if err != nil {
		return ModelFile{}, err
	}
	total, err := modelBytes(path)
	if err != nil {
		return ModelFile{}, err
	}
	return ModelFile{Metadata: metadata, FileBytes: total}, nil
}

func modelBytes(path string) (int64, error) {
	shards := splitShardPaths(path)
	var total int64
	for _, shard := range shards {
		info, err := os.Stat(shard)
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func splitShardPaths(path string) []string {
	match := splitShardPattern.FindStringSubmatch(filepath.Base(path))
	if match == nil {
		return []string{path}
	}
	count, err := strconv.Atoi(match[3])
	if err != nil || count < 1 || count > maxSplitShards {
		return []string{path}
	}
	directory := filepath.Dir(path)
	shards := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		shards = append(shards, filepath.Join(directory, fmt.Sprintf("%s-%05d-of-%05d.gguf", match[1], index, count)))
	}
	return shards
}
