package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"tensors-router/internal/tufpublish"
	"time"
)

func main() {
	configPath := flag.String("config", "tuf/upstreams.json", "upstream selection configuration")
	repositoryPath := flag.String("repository", "published", "current public TUF repository")
	outputPath := flag.String("output", ".tmp/tuf-publication", "fresh publication output")
	flag.Parse()
	content, err := os.ReadFile(*configPath)
	if err != nil {
		fail(err)
	}
	var config tufpublish.Config
	if err := json.Unmarshal(content, &config); err != nil {
		fail(err)
	}
	targets, err := tufpublish.Discover(context.Background(), &http.Client{Timeout: 30 * time.Minute}, config)
	if err != nil {
		fail(err)
	}
	err = tufpublish.Publish(*repositoryPath, *outputPath, targets, tufpublish.SigningSecrets{Upstream: os.Getenv("TUF_UPSTREAM_TARGETS_KEY"), Snapshot: os.Getenv("TUF_SNAPSHOT_KEY"), Timestamp: os.Getenv("TUF_TIMESTAMP_KEY")}, time.Now().UTC())
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
