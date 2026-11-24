package webui

import (
	"os"
	"time"
)

type fileStamp struct {
	present bool
	size    int64
	modTime time.Time
}

func statFile(path string) fileStamp {
	if path == "" {
		return fileStamp{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{present: true, size: info.Size(), modTime: info.ModTime()}
}
