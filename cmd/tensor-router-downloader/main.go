package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"tensors-router/internal/downloader"
)

const configFlag = "--config"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func runWorker(args []string, input io.Reader, output io.Writer) error {
	if len(args) != 2 || args[0] != configFlag || strings.TrimSpace(args[1]) == "" {
		return fmt.Errorf("worker requires --config PATH")
	}
	config, _, err := downloader.LoadConfig(args[1])
	if err != nil {
		return err
	}
	return downloader.ServeWorker(config, input, output)
}

type downloadCommand struct {
	repository string
	files      []string
	revision   string
	configPath string
	mode       string
	yes        bool
}

func printPlan(output io.Writer, plan downloader.DownloadPlan) error {
	if _, err := fmt.Fprintf(output, "Repository: %s\nCommit: %s\nDestination: %s\nTotal: %d bytes\n", plan.Repository, plan.Commit, plan.Destination, plan.TotalBytes); err != nil {
		return err
	}
	for _, file := range plan.Files {
		if _, err := fmt.Fprintf(output, "  %s (%d bytes, %s)\n", file.Path, file.Size, file.Reason); err != nil {
			return err
		}
	}
	return nil
}

func rangeJobEvents(manager *downloader.Manager, id string) <-chan downloader.DownloadJob {
	result := make(chan downloader.DownloadJob)
	events, unsubscribe := manager.Subscribe(id)
	go func() {
		defer close(result)
		defer unsubscribe()
		for event := range events {
			result <- event
			if event.State == downloader.JobCompleted || event.State == downloader.JobFailed || event.State == downloader.JobCancelled {
				return
			}
		}
	}()
	return result
}

func usage(output io.Writer) error {
	_, err := fmt.Fprintln(output, "Usage: tensor-router-downloader download REPO FILE... [--revision REVISION] --config downloader.yaml [--yes]\n       tensor-router-downloader download REPO --all [--revision REVISION] --config downloader.yaml [--yes]")
	return err
}
