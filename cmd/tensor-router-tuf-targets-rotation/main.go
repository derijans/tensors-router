package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"tensors-router/internal/tufpublish"
)

func main() {
	if len(os.Args) < 2 {
		fail(fmt.Errorf("expected prepare or install command"))
	}
	switch os.Args[1] {
	case "prepare":
		prepare(os.Args[2:])
	case "install":
		install(os.Args[2:])
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func prepare(arguments []string) {
	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	repository := flags.String("repository", "published", "current public TUF repository")
	output := flags.String("output", ".tmp/tuf-targets-rotation-request", "empty output directory for public signing material")
	expiresValue := flags.String("expires", "", "new targets expiration in RFC3339 format")
	if err := flags.Parse(arguments); err != nil {
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(fmt.Errorf("prepare does not accept positional arguments"))
	}
	if *expiresValue == "" {
		fail(fmt.Errorf("-expires is required"))
	}
	expires, err := time.Parse(time.RFC3339, *expiresValue)
	if err != nil {
		fail(fmt.Errorf("parse -expires: %w", err))
	}
	if err := tufpublish.PrepareTargetsRotation(*repository, *output, expires, time.Now().UTC()); err != nil {
		fail(err)
	}
}

func install(arguments []string) {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	repository := flags.String("repository", "published", "current public TUF repository")
	signedTargets := flags.String("signed-targets", "", "offline threshold-signed targets metadata")
	output := flags.String("output", ".tmp/tuf-targets-rotation-staged", "empty staging repository for online publication")
	if err := flags.Parse(arguments); err != nil {
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(fmt.Errorf("install does not accept positional arguments"))
	}
	if *signedTargets == "" {
		fail(fmt.Errorf("-signed-targets is required"))
	}
	if err := tufpublish.InstallTargetsRotation(*repository, *signedTargets, *output, time.Now().UTC()); err != nil {
		fail(err)
	}
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
