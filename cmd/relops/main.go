package main

import (
	"os"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/launcher"
)

var version = "relops-launcher-version:dev:end"

func main() {
	os.Exit(launcher.Main(os.Args[1:], strings.TrimSuffix(strings.TrimPrefix(version, "relops-launcher-version:"), ":end")))
}
