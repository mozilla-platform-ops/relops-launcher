package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/sitebuild"
)

func main() {
	version, err := os.ReadFile("VERSION")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read VERSION:", err)
		os.Exit(1)
	}
	cfg := sitebuild.Config{}
	flag.StringVar(&cfg.Source, "source", "quick_site", "site source directory")
	flag.StringVar(&cfg.Binaries, "binaries", "dist", "directory containing release binaries; output is its site/ subdirectory")
	flag.StringVar(&cfg.Version, "version", strings.TrimSpace(string(version)), "release version")
	flag.BoolVar(&cfg.RequireNotarized, "require-notarized", false, "require Apple acceptance matching the exact macOS artifacts")
	flag.Parse()
	output, err := sitebuild.Build(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prepare site:", err)
		os.Exit(1)
	}
	fmt.Println("Prepared", output)
}
