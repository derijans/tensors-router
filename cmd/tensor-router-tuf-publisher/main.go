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
	runtimeEvidencePath := flag.String("runtime-evidence", "", "validated vLLM runtime evidence directory")
	sourceCommit := flag.String("source-commit", "", "source commit covered by runtime evidence")
	flag.Parse()
	content, err := os.ReadFile(*configPath)
	if err != nil {
		fail(err)
	}
	var config tufpublish.Config
	if err := json.Unmarshal(content, &config); err != nil {
		fail(err)
	}
	if len(config.RuntimeManifests) != 0 {
		fail(fmt.Errorf("runtime manifests must be supplied through validated runtime evidence"))
	}
	now := time.Now().UTC()
	if *runtimeEvidencePath != "" {
		manifestPaths, err := tufpublish.LoadRuntimeEvidence(*runtimeEvidencePath, *sourceCommit, now)
		if err != nil {
			fail(err)
		}
		config.RuntimeManifests = manifestPaths
	}
	targets, err := tufpublish.Discover(context.Background(), &http.Client{Timeout: 30 * time.Minute}, config)
	if err != nil {
		fail(err)
	}
	if *runtimeEvidencePath == "" {
		existingTargets, err := tufpublish.LoadExistingTargets(*repositoryPath, "runtimes/vllm/")
		if err != nil {
			fail(err)
		}
		for targetPath, body := range existingTargets {
			targets[targetPath] = body
		}
	}
	err = tufpublish.Publish(*repositoryPath, *outputPath, targets, tufpublish.SigningSecrets{Upstream: os.Getenv("TUF_UPSTREAM_TARGETS_KEY"), Snapshot: os.Getenv("TUF_SNAPSHOT_KEY"), Timestamp: os.Getenv("TUF_TIMESTAMP_KEY")}, now)
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
