package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	checkTools := flag.Bool("check-tools", false, "check live catalog refs and paths using Git SSH, without building the site")
	flag.StringVar(&cfg.Source, "source", "quick_site", "site source directory")
	flag.StringVar(&cfg.Binaries, "binaries", "dist", "directory containing release binaries; output is its site/ subdirectory")
	flag.StringVar(&cfg.Version, "version", strings.TrimSpace(string(version)), "release version")
	flag.BoolVar(&cfg.RequireNotarized, "require-notarized", false, "require Apple acceptance matching the exact macOS artifacts")
	flag.Parse()
	if *checkTools {
		if err := sitebuild.CheckTools(context.Background(), filepath.Join(cfg.Source, "tools.json"), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "catalog check failed:", err)
			os.Exit(1)
		}
		return
	}
	output, err := sitebuild.Build(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prepare site:", err)
		os.Exit(1)
	}
	fmt.Println("Prepared", output)
}
