package webui

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var embeddedAssets embed.FS

func AssetFS() fs.FS {
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	return assets
}
