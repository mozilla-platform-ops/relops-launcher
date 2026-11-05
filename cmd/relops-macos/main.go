package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mozilla-platform-ops/relops-launcher/internal/macosrelease"
)

func main() {
	if runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "macOS release signing requires macOS, Xcode tools, and a Developer ID Application identity in Keychain")
		os.Exit(1)
	}
	version, err := os.ReadFile("VERSION")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg := macosrelease.Config{}
	flag.StringVar(&cfg.Dist, "dist", "dist", "release binary directory")
	flag.StringVar(&cfg.Version, "version", strings.TrimSpace(string(version)), "release version")
	flag.StringVar(&cfg.Identity, "identity", "", "Developer ID Application signing identity")
	flag.StringVar(&cfg.TeamID, "team-id", "", "Apple Developer Team ID")
	flag.StringVar(&cfg.Profile, "profile", "relops-notary", "notarytool Keychain profile name")
	flag.DurationVar(&cfg.Timeout, "timeout", 20*time.Minute, "notarization wait timeout; rerun to resume after timeout")
	flag.BoolVar(&cfg.SignOnly, "sign-only", false, "sign without submitting to Apple (not sufficient for distribution)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := macosrelease.Run(ctx, cfg, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "macOS release:", err)
		os.Exit(1)
	}
}
