package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/config"
	routerupdate "tensors-router/internal/update"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "download":
		return runDownload(args[1:])
	case "benchmark":
		return runBenchmark(args[1:])
	case "version":
		fmt.Println(buildinfo.Current())
		return nil
	case "-h", "--help", "help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func configuredLoggers(mode string) (*log.Logger, *log.Logger) {
	discard := log.New(io.Discard, "", 0)
	switch mode {
	case config.LoggingModeNormal:
		return log.Default(), log.Default()
	case config.LoggingModeStartupOnly:
		return log.Default(), discard
	default:
		return discard, discard
	}
}

func bearerAuthConfigured(keys []string) bool {
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			return true
		}
	}
	return false
}

func runDownload(args []string) error {
	flags := flag.NewFlagSet("download", flag.ContinueOnError)
	configPath := flags.String("config", "config.yaml", "config file")
	securityProfile := flags.String("security-profile", "", "security profile")
	if err := flags.Parse(args); err != nil {
		return err
	}

	profileOverride := config.ResolveSecurityProfile(*securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	loadOptions, err := environmentLoadOptions(profileOverride)
	if err != nil {
		return err
	}
	cfg, err := config.LoadWithOptions(*configPath, loadOptions)
	if err != nil {
		return err
	}

	updater := routerupdate.NewManager(cfg)
	paths, err := updater.DownloadedPaths(context.Background())
	if err != nil {
		return err
	}

	for _, path := range paths {
		fmt.Printf("downloaded %s\n", path)
	}
	return nil
}

func usage() error {
	fmt.Println("usage:")
	fmt.Println("  tensors-router serve --config config.yaml")
	fmt.Println("  tensors-router download --config config.yaml")
	fmt.Println("  tensors-router benchmark --model model-id")
	fmt.Println("  tensors-router version")
	return nil
}
