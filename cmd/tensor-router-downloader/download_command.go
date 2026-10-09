package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/downloader"
)

func run(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		_, _ = fmt.Fprintln(output, buildinfo.Current())
		return nil
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return usage(output)
	}
	switch args[0] {
	case "worker":
		return runWorker(args[1:], input, output)
	case "download":
		return runDownload(args[1:], input, output)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runDownload(args []string, input io.Reader, output io.Writer) error {
	command, err := parseDownloadCommand(args)
	if err != nil {
		return err
	}
	manager, err := openDownloadManager(command.configPath, output)
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
	if plan.AlreadyPresent() {
		_, _ = fmt.Fprintln(output, "every planned file is already present with the same hash")
		return nil
	}
	if plan.UnsafeWarning {
		_, _ = fmt.Fprintln(output, "warning: Hugging Face reports an unsafe or pending repository security status")
	}
	if !command.yes {
		if err := confirmDownload(input, output); err != nil {
			return err
		}
	}
	job, err := manager.CreatePlannedJob(plan, "", true, true)
	if err != nil {
		return err
	}
	return followDownload(manager, job.ID, output)
}

func openDownloadManager(configPath string, output io.Writer) (*downloader.Manager, error) {
	config, warnings, err := downloader.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(output, "warning: %s\n", warning)
	}
	return downloader.NewManager(config, "")
}

func confirmDownload(input io.Reader, output io.Writer) error {
	_, _ = fmt.Fprint(output, "Continue? [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("download cancelled")
	}
	return nil
}

func followDownload(manager *downloader.Manager, jobID string, output io.Writer) error {
	for event := range rangeJobEvents(manager, jobID) {
		_, _ = fmt.Fprintf(output, "%s %d/%d bytes\n", event.State, event.CompletedBytes, event.TotalBytes)
		switch event.State {
		case downloader.JobCompleted:
			return nil
		case downloader.JobFailed, downloader.JobCancelled:
			return fmt.Errorf("download %s: %s", event.State, event.Error)
		}
	}
	return fmt.Errorf("download event stream closed")
}

func parseDownloadCommand(args []string) (downloadCommand, error) {
	command, values, err := parseDownloadFlags(args)
	if err != nil {
		return downloadCommand{}, err
	}
	if len(values) == 0 {
		return downloadCommand{}, fmt.Errorf("repository is required")
	}
	command.repository = values[0]
	command.files = values[1:]
	if err := validateDownloadCommand(command); err != nil {
		return downloadCommand{}, err
	}
	return command, nil
}

func parseDownloadFlags(args []string) (downloadCommand, []string, error) {
	command := downloadCommand{configPath: "downloader.yaml", mode: "smart"}
	values := []string{}
	for index := 0; index < len(args); index++ {
		value := args[index]
		switch value {
		case configFlag, "--revision":
			if index+1 >= len(args) {
				return downloadCommand{}, nil, fmt.Errorf("%s requires a value", value)
			}
			index++
			if value == configFlag {
				command.configPath = args[index]
			} else {
				command.revision = args[index]
			}
		case "--all":
			command.mode = "snapshot"
		case "--yes":
			command.yes = true
		default:
			if strings.HasPrefix(value, "-") {
				return downloadCommand{}, nil, fmt.Errorf("unknown flag %q", value)
			}
			values = append(values, value)
		}
	}
	return command, values, nil
}

func validateDownloadCommand(command downloadCommand) error {
	if command.mode == "snapshot" && len(command.files) > 0 {
		return fmt.Errorf("--all cannot be combined with explicit files")
	}
	if command.mode != "snapshot" && len(command.files) == 0 {
		return fmt.Errorf("one or more files are required unless --all is used")
	}
	if err := downloader.ValidateRepository(command.repository); err != nil {
		return err
	}
	for _, file := range command.files {
		if err := downloader.ValidateRepositoryPath(file); err != nil {
			return err
		}
	}
	return nil
}
