package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"tensors-router/internal/tufpublish"
)

func main() {
	evidencePath := flag.String("evidence", "", "runtime profile evidence directory")
	sourceCommit := flag.String("source-commit", "", "source commit covered by evidence")
	flag.Parse()
	if _, err := tufpublish.LoadRuntimeEvidence(*evidencePath, *sourceCommit, time.Now().UTC()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
