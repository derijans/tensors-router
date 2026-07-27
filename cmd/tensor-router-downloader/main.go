package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/downloader"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		_, _ = fmt.Fprintln(output, buildinfo.Current())
		return nil
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return usage(output)
	}
	if args[0] != "download" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	command, err := parseDownloadCommand(args[1:])
	if err != nil {
		return err
	}
	config, warnings, err := downloader.LoadConfig(command.configPath)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(output, "warning: %s\n", warning)
	}
	manager, err := downloader.NewManager(config, "")
	if err != nil {
		return err
	}
	defer manager.Close()
	plan, err := manager.Plan(context.Background(), downloader.PlanRequest{Repository: command.repository, Revision: command.revision, Files: command.files, Mode: command.mode})
	if err != nil {
		return err
	}
	if err := printPlan(output, plan); err != nil {
		return err
	}
	if plan.UnsafeWarning {
		_, _ = fmt.Fprintln(output, "warning: Hugging Face reports an unsafe or pending repository security status")
	}
	if !command.yes {
		_, _ = fmt.Fprint(output, "Continue? [y/N] ")
		line, err := bufio.NewReader(input).ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		if strings.ToLower(strings.TrimSpace(line)) != "y" && strings.ToLower(strings.TrimSpace(line)) != "yes" {
			return fmt.Errorf("download cancelled")
		}
	}
	job, err := manager.CreatePlannedJob(plan, "", true, true)
	if err != nil {
		return err
	}
	for event := range rangeJobEvents(manager, job.ID) {
		_, _ = fmt.Fprintf(output, "%s %d/%d bytes\n", event.State, event.CompletedBytes, event.TotalBytes)
		if event.State == downloader.JobCompleted {
			return nil
		}
		if event.State == downloader.JobFailed || event.State == downloader.JobCancelled {
			return fmt.Errorf("download %s: %s", event.State, event.Error)
		}
	}
	return fmt.Errorf("download event stream closed")
}

type downloadCommand struct {
	repository string
	files      []string
	revision   string
	configPath string
	mode       string
	yes        bool
}

func parseDownloadCommand(args []string) (downloadCommand, error) {
	command := downloadCommand{configPath: "downloader.yaml", mode: "smart"}
	values := []string{}
	for index := 0; index < len(args); index++ {
		value := args[index]
		switch value {
		case "--config", "--revision":
			if index+1 >= len(args) {
				return downloadCommand{}, fmt.Errorf("%s requires a value", value)
			}
			index++
			if value == "--config" {
				command.configPath = args[index]
			} else {
				command.revision = args[index]
			}
		case "--all":
			if command.mode == "explicit" {
				return downloadCommand{}, fmt.Errorf("--all cannot be combined with explicit files")
			}
			command.mode = "snapshot"
		case "--yes":
			command.yes = true
		default:
			if strings.HasPrefix(value, "-") {
				return downloadCommand{}, fmt.Errorf("unknown flag %q", value)
			}
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return downloadCommand{}, fmt.Errorf("repository is required")
	}
	command.repository = values[0]
	command.files = values[1:]
	if command.mode == "snapshot" && len(command.files) > 0 {
		return downloadCommand{}, fmt.Errorf("--all cannot be combined with explicit files")
	}
	if command.mode != "snapshot" && len(command.files) == 0 {
		return downloadCommand{}, fmt.Errorf("one or more files are required unless --all is used")
	}
	if err := downloader.ValidateRepository(command.repository); err != nil {
		return downloadCommand{}, err
	}
	for _, file := range command.files {
		if err := downloader.ValidateRepositoryPath(file); err != nil {
			return downloadCommand{}, err
		}
	}
	return command, nil
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
